package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"github.com/ArchiMoebius/fishler/cli/config"
	gossh "golang.org/x/crypto/ssh"
)

func GetKeySigner() (gossh.Signer, error) {

	privatekey, err := GetFishlerPrivateKey()
	if err != nil {
		Logger.Error(err)
		return nil, err
	}

	pemBytes, err := os.ReadFile(privatekey)
	if err != nil {
		Logger.Error(err)
		return nil, err
	}

	signer, err := gossh.ParsePrivateKey(pemBytes)
	if err != nil {
		Logger.Error(err)
		return nil, err
	}

	return signer, nil
}

func GetFishlerPrivateKey() (string, error) {
	if _, err := os.Stat(config.GlobalConfig.PrivateKeyFilepath); errors.Is(err, os.ErrNotExist) {
		if err := generateKey(); err != nil {
			return "", err
		}
	}

	return config.GlobalConfig.PrivateKeyFilepath, nil
}

func generateKey() error {
	// Generate a new RSA private key with 2048 bits
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		Logger.Error(err)
		return err
	}

	// Encode the private key to the PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyFile, err := os.Create(config.GlobalConfig.PrivateKeyFilepath)
	if err != nil {
		Logger.Error(err)
		return err
	}
	pem.Encode(privateKeyFile, privateKeyPEM)
	privateKeyFile.Close()

	// Extract the public key from the private key
	publicKey := &privateKey.PublicKey

	// Encode the public key to the PEM format
	publicKeyPEM := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(publicKey),
	}
	publicKeyFile, err := os.Create(fmt.Sprintf("%s.pub", config.GlobalConfig.PrivateKeyFilepath))
	if err != nil {
		Logger.Error(err)
		return err
	}
	pem.Encode(publicKeyFile, publicKeyPEM)
	publicKeyFile.Close()

	return nil
}
