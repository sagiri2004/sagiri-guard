package auth

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func PromptCredentials() (string, string, error) {
	in := bufio.NewReader(os.Stdin)
	fmt.Print("Username: ")
	u, _ := in.ReadString('\n')
	fmt.Print("Password: ")
	p, _ := in.ReadString('\n')
	u = strings.TrimSpace(u)
	p = strings.TrimSpace(p)
	if u == "" || p == "" {
		return "", "", fmt.Errorf("missing credentials")
	}
	return u, p, nil
}
