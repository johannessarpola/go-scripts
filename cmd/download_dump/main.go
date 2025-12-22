package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	ssht "github.com/johannessarpola/go-scripts/internal/ssh_tunnel"
	"github.com/schollz/progressbar/v3"
)

type DumperConfig struct {
	outputDumpFile   string
	postgresUser     string
	postgresPassword string
	postgresHost     string
	postgresPort     string
	postgresDB       string
}

type SshTunnelConfig struct {
	sshAddr           string
	sshUser           string
	sshPrivateKeyFile string
	sshDestination    string
}

func (s *SshTunnelConfig) sshConnectionAddress() string {
	return fmt.Sprintf("%s@%s", s.sshUser, s.sshAddr)
}

// ProgressWriter wraps an io.Writer and updates a progress bar
type ProgressWriter struct {
	writer io.Writer
	bar    *progressbar.ProgressBar
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if err == nil {
		pw.bar.Add(n)
	}
	return n, err
}

func main() {
	dumpCmd := os.Getenv("POSTGRES_DUMP_CMD")
	if dumpCmd == "" {
		dumpCmd = "pg_dump" // default
	}

	dumpConfig := DumperConfig{
		postgresUser:     os.Getenv("POSTGRES_USER"),
		postgresPassword: os.Getenv("POSTGRES_PASSWORD"),
		postgresHost:     os.Getenv("POSTGRES_HOST"),
		postgresPort:     os.Getenv("POSTGRES_PORT"),
		postgresDB:       os.Getenv("POSTGRES_DB"),
		outputDumpFile:   os.Getenv("OUTPUT_DUMP"),
	}

	sshConfig := SshTunnelConfig{
		sshUser:           os.Getenv("SSH_USER"),
		sshPrivateKeyFile: os.Getenv("SSH_PRIVATE_KEY_FILE"),
		sshAddr:           os.Getenv("SSH_ADDR"),
		sshDestination:    os.Getenv("SSH_DESTINATION"),
	}

	to := 60 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), to)
	defer cancel()

	tunnel := configureTunnel(&sshConfig)

	// Channel to signal tunnel is ready
	tunnelReady := make(chan error, 1)
	go func() {
		tunnelReady <- tunnel.Start(ctx)
	}()

	// Wait for tunnel to be ready with a shorter timeout
	select {
	case err := <-tunnelReady:
		if err != nil {
			log.Fatalf("Failed to start tunnel: %v", err)
		}
	case <-time.After(5 * time.Second):
		// Tunnel should be up by now
	}

	// Query database size first
	fmt.Println("Querying database size...")
	dbSize, err := getDatabaseSize(ctx, &dumpConfig, tunnel.Local.Port)
	if err != nil {
		log.Printf("Warning: couldn't get database size: %v, continuing with unknown size", err)
		dbSize = -1 // unknown size
	} else {
		fmt.Printf("Database size: %s\n", formatBytes(dbSize))
	}

	// Create output file first
	outputFile, err := os.Create(dumpConfig.outputDumpFile)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer outputFile.Close()

	// Create buffered writer
	bw := bufio.NewWriterSize(outputFile, 256*1024) // 256KB buffer
	defer bw.Flush()

	// Write header
	if _, err := bw.WriteString("-- Output generated with pg_dump script\n"); err != nil {
		log.Fatalf("Failed to write header: %v", err)
	}

	// Create command with context for timeout handling
	cmd := exec.CommandContext(ctx, dumpCmd,
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", tunnel.Local.Port),
		"-U", dumpConfig.postgresUser,
		"--no-password",
		dumpConfig.postgresDB,
	)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", dumpConfig.postgresPassword))

	// Stream stdout directly to file
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to create stdout pipe: %v", err)
	}

	// Capture stderr for error reporting
	cmd.Stderr = os.Stderr

	fmt.Printf("Starting dump command: %s %v\n", dumpCmd, cmd.Args[1:])

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start command: %v", err)
	}

	// Create progress bar
	var bar *progressbar.ProgressBar
	if dbSize > 0 {
		// Known size - show percentage
		bar = progressbar.NewOptions64(dbSize,
			progressbar.OptionSetDescription("Dumping database"),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetWidth(20),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() {
				fmt.Fprint(os.Stderr, "\n")
			}),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
			progressbar.OptionEnableColorCodes(true),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
	} else {
		// Unknown size - show spinner
		bar = progressbar.NewOptions(-1,
			progressbar.OptionSetDescription("Dumping database"),
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetWidth(15),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() {
				fmt.Fprint(os.Stderr, "\n")
			}),
			progressbar.OptionSpinnerType(14),
			progressbar.OptionFullWidth(),
			progressbar.OptionSetRenderBlankState(true),
		)
	}

	// Wrap buffered writer with progress tracking
	pw := &ProgressWriter{
		writer: bw,
		bar:    bar,
	}

	// Stream the output to file with progress tracking
	written, err := io.Copy(pw, stdout)
	if err != nil {
		log.Fatalf("Failed to copy output: %v", err)
	}

	// Wait for command to complete
	if err := cmd.Wait(); err != nil {
		log.Fatalf("Command failed: %v", err)
	}

	// Ensure everything is flushed
	if err := bw.Flush(); err != nil {
		log.Fatalf("Failed to flush output: %v", err)
	}

	bar.Finish()

	fmt.Printf("Successfully dumped database (%s) to %s\n", formatBytes(written), dumpConfig.outputDumpFile)
}

// getDatabaseSize queries PostgreSQL for the size of the database
func getDatabaseSize(ctx context.Context, config *DumperConfig, port int) (int64, error) {
	sizeCmd := exec.CommandContext(ctx, "psql",
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", port),
		"-U", config.postgresUser,
		"--no-password",
		"-t", // tuples only (no headers)
		"-c", fmt.Sprintf("SELECT pg_database_size('%s')", config.postgresDB),
	)
	sizeCmd.Env = append(os.Environ(), fmt.Sprintf("PGPASSWORD=%s", config.postgresPassword))

	output, err := sizeCmd.Output()
	if err != nil {
		return -1, fmt.Errorf("failed to query database size: %w", err)
	}

	// Parse the output (it will be something like "  123456789\n")
	sizeStr := strings.TrimSpace(string(output))
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return -1, fmt.Errorf("failed to parse database size '%s': %w", sizeStr, err)
	}

	return size, nil
}

// formatBytes formats bytes in human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func configureTunnel(conf *SshTunnelConfig) *ssht.SSHTunnel {
	pk := ssht.NewPrivateKey(conf.sshPrivateKeyFile)
	tunnel := ssht.NewSSHTunnel(
		conf.sshConnectionAddress(),
		pk,
		conf.sshDestination,
	)

	tunnel.Log = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds)

	return tunnel
}
