package crypto

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	passphraseEnvVar       = "DOT_VAULT_MASTER_PASSPHRASE"
	passphraseIterations   = 120000
	passphraseSaltPrefix   = "dot-vault/master-key/v1/"
	minPassphraseByteCount = 12
)

type PassphraseProvider struct {
	in          *os.File
	err         io.Writer
	passphrases map[string]string
	interactive bool
}

func NewPassphraseProvider(in *os.File, err io.Writer) *PassphraseProvider {
	if in == nil {
		in = os.Stdin
	}
	if err == nil {
		err = os.Stderr
	}
	return &PassphraseProvider{
		in:          in,
		err:         err,
		passphrases: map[string]string{},
		interactive: true,
	}
}

func (p *PassphraseProvider) GetOrCreateMasterKey(_ context.Context, organization string) ([]byte, error) {
	organization = strings.TrimSpace(organization)
	if organization == "" {
		return nil, fmt.Errorf("organization is required")
	}

	passphrase, err := p.readPassphrase(organization)
	if err != nil {
		return nil, err
	}
	if len([]byte(passphrase)) < minPassphraseByteCount {
		return nil, fmt.Errorf("master passphrase must be at least %d bytes", minPassphraseByteCount)
	}

	return derivePassphraseMasterKey(organization, passphrase), nil
}

func (p *PassphraseProvider) HasPassphrase(organization string) bool {
	organization = strings.TrimSpace(organization)
	if organization == "" {
		return false
	}
	if passphrase := p.passphrases[organization]; strings.TrimSpace(passphrase) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv(passphraseEnvVar)) != ""
}

func (p *PassphraseProvider) SetPassphrase(organization string, passphrase string) error {
	organization = strings.TrimSpace(organization)
	if organization == "" {
		return fmt.Errorf("organization is required")
	}
	if len([]byte(passphrase)) < minPassphraseByteCount {
		return fmt.Errorf("master passphrase must be at least %d bytes", minPassphraseByteCount)
	}
	p.passphrases[organization] = passphrase
	return nil
}

func (p *PassphraseProvider) SetInteractive(interactive bool) {
	p.interactive = interactive
}

func (p *PassphraseProvider) readPassphrase(organization string) (string, error) {
	if passphrase := p.passphrases[organization]; strings.TrimSpace(passphrase) != "" {
		return passphrase, nil
	}
	if passphrase := os.Getenv(passphraseEnvVar); strings.TrimSpace(passphrase) != "" {
		return passphrase, nil
	}
	if !p.interactive {
		return "", fmt.Errorf("master passphrase is not unlocked for %s; set %s or restart setup", organization, passphraseEnvVar)
	}
	if !isTerminal(p.in) {
		return "", fmt.Errorf("%s is required when keyring access is unavailable in a non-interactive session", passphraseEnvVar)
	}

	fmt.Fprintf(p.err, "Master passphrase for %s: ", organization)
	payload, err := bufio.NewReader(p.in).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read master passphrase: %w", err)
	}
	return strings.TrimRight(payload, "\r\n"), nil
}

func derivePassphraseMasterKey(organization string, passphrase string) []byte {
	salt := []byte(passphraseSaltPrefix + organization)
	return pbkdf2SHA256([]byte(passphrase), salt, passphraseIterations, masterKeySize)
}

func pbkdf2SHA256(password []byte, salt []byte, iterations int, keyLength int) []byte {
	hashLength := sha256.Size
	blockCount := (keyLength + hashLength - 1) / hashLength
	derived := make([]byte, 0, blockCount*hashLength)

	for block := 1; block <= blockCount; block++ {
		derived = append(derived, pbkdf2BlockSHA256(password, salt, iterations, block)...)
	}
	return derived[:keyLength]
}

func pbkdf2BlockSHA256(password []byte, salt []byte, iterations int, block int) []byte {
	blockSalt := make([]byte, len(salt)+4)
	copy(blockSalt, salt)
	binary.BigEndian.PutUint32(blockSalt[len(salt):], uint32(block))

	u := hmacSHA256(password, blockSalt)
	out := make([]byte, len(u))
	copy(out, u)

	for i := 1; i < iterations; i++ {
		u = hmacSHA256(password, u)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}

func hmacSHA256(key []byte, payload []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil)
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
