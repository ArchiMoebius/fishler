package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"syscall"

	config "github.com/archimoebius/fishler/cli/config/serve"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func GetKeySigner() (gossh.Signer, error) {

	privatekey, err := GetFishlerPrivateKey()
	if err != nil {
		Logger.Errorf("Unable to read or create file %s make sure the directories exist and have the correct permissions", GetFishlerPrivateKeyPath())
		Logger.Fatal(err)
		return nil, err
	}

	pemBytes, err := os.ReadFile(privatekey) // #nosec
	if err != nil {
		Logger.Error(err)
		return nil, err
	}

	signer, err := gossh.ParsePrivateKey(pemBytes)
	if err != nil {

		if strings.Contains(err.Error(), "protected") {

			fmt.Printf("\n\n%s Password: ", privatekey)

			bytePassword, err := term.ReadPassword(int(syscall.Stdin))
			fmt.Printf("\n")

			if err != nil {
				Logger.Fatal(err)
				return nil, err
			}

			signer, err = gossh.ParsePrivateKeyWithPassphrase(pemBytes, bytePassword)

			if err != nil {
				Logger.Fatal(err)
				return nil, err
			}
		}
	}

	return signer, nil
}

func GetFishlerPrivateKeyPath() string {
	err := os.MkdirAll(config.Setting.CryptoBasepath, 0750)
	if err != nil {
		log.Fatal(err)
	}

	return fmt.Sprintf("%s/id_rsa", config.Setting.CryptoBasepath)
}

func GetFishlerPrivateKey() (string, error) {
	_, err := os.Stat(GetFishlerPrivateKeyPath())

	if err != nil {

		if errors.Is(err, os.ErrNotExist) {
			if err = generateKey(); err != nil {
				return "", err
			}
		}
	}

	if err != nil {
		return "", err
	}

	return GetFishlerPrivateKeyPath(), nil
}

func generateKey() error {
	// Generate a new RSA private key with 2048 bits
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	// Encode the private key to the PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyFile, err := os.Create(GetFishlerPrivateKeyPath())
	if err != nil {
		return err
	}
	err = pem.Encode(privateKeyFile, privateKeyPEM)
	if err != nil {
		return err
	}
	err = privateKeyFile.Close()
	if err != nil {
		return err
	}

	// Extract the public key from the private key
	publicKey := &privateKey.PublicKey

	// Encode the public key to the PEM format
	publicKeyPEM := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(publicKey),
	}
	publicKeyFile, err := os.Create(fmt.Sprintf("%s.pub", GetFishlerPrivateKeyPath()))
	if err != nil {
		return err
	}
	err = pem.Encode(publicKeyFile, publicKeyPEM)
	if err != nil {
		return err
	}
	err = publicKeyFile.Close()
	if err != nil {
		return err
	}

	return nil
}
