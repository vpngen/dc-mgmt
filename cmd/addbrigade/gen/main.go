package main

import (
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/vpngen/wordsgens/namesgenerator"
)

func main() {
	id := uuid.New()
	brigadierID := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:])

	fullname, person, err := namesgenerator.PhysicsAwardee()
	if err != nil {
		log.Fatalf("Can't generate: %s\n", err)
	}

	brigadierName := base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(fullname))
	personName := base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.Name))
	personDesc := base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.Desc))
	personURL := base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.URL))

	fmt.Printf("%s %s %s %s %s\n", brigadierID, brigadierName, personName, personDesc, personURL)
}
