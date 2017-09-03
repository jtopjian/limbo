package lib

import (
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/scrypt"
)

// This is taken exclusively from
// https://godoc.org/golang.org/x/crypto/nacl/secretbox
// https://github.com/danderson/gobox

func Encrypt(file, pass string) error {
	f, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("Unable to read file %s: %s", file, err)
	}

	out := make([]byte, 24+24+len(f)+secretbox.Overhead)
	if _, err = io.ReadFull(rand.Reader, out[:24]); err != nil {
		return fmt.Errorf("Unable to generate random salt")
	}

	s, err := scrypt.Key([]byte(pass), out[:24], 16384, 8, 1, 32)
	if err != nil {
		return fmt.Errorf("Unable to scrypt pass: %s", err)
	}
	var secretKey [32]byte
	copy(secretKey[:], s)

	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return fmt.Errorf("Unable to generate nonce: %s", err)
	}
	copy(out[24:48], nonce[:])

	secretbox.Seal(out[:48], f, &nonce, &secretKey)

	newFile := file + ".nacl"
	if err := ioutil.WriteFile(newFile, out, 0644); err != nil {
		return fmt.Errorf("Unable to write encrypted file: %s", err)
	}

	if err := os.Rename(newFile, file); err != nil {
		return fmt.Errorf("Unable to rename encrypted file: %s", err)
	}

	return nil
}

func Decrypt(file, pass string) error {
	f, err := ioutil.ReadFile(file)
	if err != nil {
		return fmt.Errorf("Unable to read file %s: %s", file, err)
	}

	if len(f) < 48 {
		return fmt.Errorf("Encrypted file %s is malformed", file)
	}

	s, err := scrypt.Key([]byte(pass), f[:24], 16384, 8, 1, 32)
	if err != nil {
		return fmt.Errorf("Unable to derive encryption key: %s", err)
	}

	var secretKey [32]byte
	copy(secretKey[:], s)

	var nonce [24]byte
	copy(nonce[:], f[24:48])

	newFile := file + ".plain"
	plain, ok := secretbox.Open(nil, f[48:], &nonce, &secretKey)
	if !ok {
		return fmt.Errorf("Unable to decrypt file %s", file)
	}

	err = ioutil.WriteFile(newFile, plain, 0640)
	if err != nil {
		return fmt.Errorf("Unable to save decrypted file: %s", err)
	}

	if err := os.Rename(newFile, file); err != nil {
		return fmt.Errorf("Unable to rename decrypted file: %s", err)
	}

	return nil
}
