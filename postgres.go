package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type PostgresContainerParams struct {
	ContainerName       string
	ContainerPort       int
	NetworkName         string
	PgUser              string
	PgPassword          string
	PgDatabase          string
	DumpFilePath        string
	DumpFileInContainer string
}

// You could use this also https://podman.io/blogs/2020/08/10/podman-go-bindings.html#list-images-

var (
	dockerCmd         string
	postgresContainer = "postgres:16"
)

func main() {
	// Define command-line flags
	containerName := flag.String("container", "postgres", "Name of the Docker container")
	containerNameShort := flag.String("c", "postgres", "Name of the Docker container (shorthand)")

	containerPort := flag.Int("port", 5432, "Port of the Docker container")
	containerPortShort := flag.Int("p", 5432, "Port of the Docker container (shorthand)")

	dockerCmdFlag := flag.String("docker", "docker", "Command to use for Docker")
	dockerCmdShortFlag := flag.String("d", "docker", "Command to use for Docker (shorthand)")

	// Parse the flags
	flag.Parse()

	// Use the provided container name or default
	if *containerNameShort != "postgres" {
		containerName = containerNameShort
	}

	// Use the provided container_port or default
	if *containerPortShort != 5432 {
		containerPort = containerPortShort
	}

	// Use the provided docker command or default
	if *dockerCmdShortFlag != "docker" {
		dockerCmd = *dockerCmdShortFlag
	} else {
		dockerCmd = *dockerCmdFlag
	}

	networkName := os.Getenv("NETWORK_NAME")
	pgUser := os.Getenv("PG_USER")
	pgPassword := os.Getenv("PG_PASSWORD")
	pgDatabase := os.Getenv("PG_DATABASE")
	dumpFilePath := os.Getenv("DUMP_FILE_PATH")
	dumpFileInContainer := os.Getenv("DUMP_FILE_IN_CONTAINER")

	params := PostgresContainerParams{
		ContainerName:       *containerName,
		ContainerPort:       *containerPort,
		NetworkName:         networkName,
		PgUser:              pgUser,
		PgPassword:          pgPassword,
		PgDatabase:          pgDatabase,
		DumpFilePath:        dumpFilePath,
		DumpFileInContainer: dumpFileInContainer,
	}

	setupContainer(&params)
}

func setupContainer(params *PostgresContainerParams) {
	// Create Docker network
	// if err := createDockerNetwork(params.NetworkName); err != nil {
	// 	fmt.Printf("Error creating Docker network: %v\n", err)
	// 	return
	// }

	// Run PostgreSQL Docker container
	if err := runPostgresContainer(
		params.ContainerName,
		params.NetworkName,
		params.PgUser,
		params.PgPassword,
		params.PgDatabase,
		params.ContainerPort); err != nil {
		fmt.Printf("Error running PostgreSQL container: %v\n", err)
		return
	}

	// Copy dump file to container
	if err := copyFileToContainer(params.ContainerName,
		params.DumpFilePath,
		params.DumpFileInContainer); err != nil {
		fmt.Printf("Error copying file to container: %v\n", err)
		return
	}

	// Restore database dump
	if err := restoreDatabaseDump(
		params.ContainerName,
		params.PgUser,
		params.PgDatabase,
		params.DumpFileInContainer,
		5,
		time.Second*30,
	); err != nil {
		fmt.Printf("Error restoring database dump: %v\n", err)
		return
	}

	fmt.Println("PostgreSQL container is running and database has been restored.")
}

func createDockerNetwork(networkName string) error {
	cmd := exec.Command("docker", "network", "create", networkName)
	return cmd.Run()
}

func runPostgresContainer(containerName, networkName, pgUser, pgPassword, pgDatabase string, pgPort int) error {
	cmd := exec.Command(
		dockerCmd, "run", "--name", containerName, "--network", networkName,
		"-p", fmt.Sprintf("%d:%d", pgPort, pgPort),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", pgUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", pgPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", pgDatabase),
		"-d", postgresContainer,
	)
	return cmd.Run()
}

func copyFileToContainer(containerName, srcFilePath, destFilePath string) error {
	cmd := exec.Command(dockerCmd, "cp", srcFilePath, fmt.Sprintf("%s:%s", containerName, destFilePath))
	return cmd.Run()
}

func restoreDatabaseDump(containerName, pgUser, pgDatabase, dumpFileInContainer string, retries int, wait time.Duration) error {
	for attempt := 0; attempt < retries; attempt++ {
		cmd := exec.Command(
			dockerCmd, "exec", "-i", containerName,
			"psql", "-U", pgUser, "-d", pgDatabase, "-f", dumpFileInContainer,
		)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err := cmd.Run()
		if err == nil {
			return nil
		}

		if attempt < retries-1 {
			fmt.Printf("Attempt %d failed: %v. Retrying in %s seconds...\n", attempt+1, err, wait)
			time.Sleep(wait)
		}
	}

	return fmt.Errorf("failed to restore database after %d attempts", retries)
}
