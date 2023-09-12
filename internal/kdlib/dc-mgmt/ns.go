package dcmgmt

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

var ErrEmptyNSString = errors.New("empty ns connect string")

func ParseNSEnv(nsenv string) ([]string, error) {
	nsString := os.Getenv(nsenv)

	if nsString == "" {
		return nil, ErrEmptyNSString
	}

	servers := strings.Split(nsString, ",")
	for _, server := range servers {
		if server == "" {
			return nil, fmt.Errorf("%w:%s", ErrInvalidServerHost, server)
		}
	}

	return servers, nil
}
