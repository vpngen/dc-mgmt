package dcmgmt

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ParseConnEnv(connenv string) (string, string, error) {
	connString := os.Getenv(connenv)

	if connString == "" {
		return "", "", ErrEmptyConnString
	}

	user, server, ok := strings.Cut(connString, "@")
	if !ok || user == "" || server == "" {
		return "", "", fmt.Errorf("%w:%s", ErrInvalidConnString, connString)
	}

	host, port, _ := strings.Cut(server, ":")
	if host == "" {
		return "", "", fmt.Errorf("%w:%s", ErrInvalidServerHost, server)
	}

	if _, err := strconv.Atoi(port); err != nil {
		return "", "", fmt.Errorf("%w:%s", ErrInvalidServerPort, port)
	}

	return user, server, nil
}
