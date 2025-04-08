package ssh_tunnel

import (
	"os"

	"golang.org/x/crypto/ssh"
)

func NewPrivateKey(file string) ssh.AuthMethod {
	buffer, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil
	}
	return ssh.PublicKeys(key)
}
