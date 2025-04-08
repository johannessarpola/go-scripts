package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	ssht "github.com/johannessarpola/go-scripts/internal/ssh_tunnel"
)

type DumperConfig struct {
	outputDumpFile   string
	postgresUser     string
	postgresPassword string // Needs to go into envvar PGPASSWORD for cmd
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

func main() {
	dumpCmd := os.Getenv("POSTGRES_DUMP_CMD")

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
	go tunnel.Start(ctx)

	time.Sleep(1 * time.Second) // Wait for a while to ensure the tunnel is up
	cmd := exec.Command(dumpCmd)
	args := []string{
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", tunnel.Local.Port),
		"-U", dumpConfig.postgresUser,
		"--no-password",
		dumpConfig.postgresDB,
	}
	cmd.Args = append(cmd.Args, args...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSWORD=%s", dumpConfig.postgresPassword))

	outputChan := make(chan []byte, 1)
	go func() {
		fmt.Printf("Starting dump command with args %v\n", cmd.Args)

		// Run the command and capture the output
		output, err := cmd.Output()
		if err != nil {
			fmt.Println("Error running command:", err)
			return
		}
		outputChan <- output
	}()

	select {
	case <-ctx.Done():
		if cmd.Process != nil {
			cmd.Process.Kill()
			fmt.Println("Command timed out and was killed")
		}
	case output := <-outputChan:
		err := writeToFile(output, dumpConfig.outputDumpFile)
		if err != nil {
			panic(err)
		}
	}
}

func writeToFile(output []byte, fileName string) error {
	outputFile, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer outputFile.Close()

	bw := bufio.NewWriter(outputFile)
	defer bw.Flush() // make sure we flush before exit
	bw.Write([]byte("-- Output generted with pg_dump script\n"))
	bw.Write(output)
	return nil
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
