package filesign

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// Signer handles file URL signing and verification
type Signer struct {
	secret        string
	publicBaseURL string
}

// NewSigner creates a new file signer
func NewSigner(secret, publicBaseURL string) *Signer {
	return &Signer{
		secret:        secret,
		publicBaseURL: publicBaseURL,
	}
}

// SignPath generates an HMAC signature for a conversation ID and filename
func (s *Signer) SignPath(conversationID, filename string) string {
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write([]byte(conversationID + "/" + filename))
	return hex.EncodeToString(mac.Sum(nil))
}

// MakeFileURL creates a signed download URL for a file
func (s *Signer) MakeFileURL(conversationID, filename string) string {
	sig := s.SignPath(conversationID, filename)
	return fmt.Sprintf("%s/files/%s/%s?sig=%s",
		strings.TrimRight(s.publicBaseURL, "/"),
		url.PathEscape(conversationID),
		url.PathEscape(filename),
		sig,
	)
}

// VerifySignature verifies a signature for a conversation ID and filename
// Uses constant-time comparison to prevent timing attacks
func (s *Signer) VerifySignature(conversationID, filename, providedSig string) bool {
	expectedSig := s.SignPath(conversationID, filename)
	return subtle.ConstantTimeCompare([]byte(expectedSig), []byte(providedSig)) == 1
}
