package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const (
	cookieName      = "dirio_console_session"
	flashCookieName = "dirio_console_flash"
	sessionDuration = 8 * time.Hour
)

// FlashData is the data stored in the flash cookie.
type FlashData struct {
	Message string `json:"msg"`
	Type    string `json:"type"`
}

// sessionPayload is the data stored (encoded + signed) in the session cookie.
type sessionPayload struct {
	AccessKey string `json:"ak"`
	ExpiresAt int64  `json:"exp"` // Unix timestamp
}

// Session manages HMAC-SHA256 signed console session cookies.
// The signing key is randomly generated at startup, so sessions are
// invalidated when the server restarts — acceptable for an admin console.
type Session struct {
	signingKey []byte
}

// NewSession creates a Session with a randomly generated signing key.
func NewSession() (*Session, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return &Session{signingKey: key}, nil
}

// Create writes a signed session cookie for the given access key.
func (s *Session) Create(w http.ResponseWriter, accessKey string) error {
	p := sessionPayload{
		AccessKey: accessKey,
		ExpiresAt: time.Now().Add(sessionDuration).Unix(),
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	encoded := hex.EncodeToString(raw)
	cookieValue := encoded + "." + s.sign(encoded)

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    cookieValue,
		Path:     "/dirio/ui/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(p.ExpiresAt, 0),
	})
	return nil
}

// Validate reads and verifies the session cookie.
// Returns the access key and true if the session is valid and unexpired.
func (s *Session) Validate(r *http.Request) (accessKey string, ok bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", false
	}

	idx := strings.LastIndex(cookie.Value, ".")
	if idx < 0 {
		return "", false
	}
	encoded, sig := cookie.Value[:idx], cookie.Value[idx+1:]

	// Constant-time signature comparison to prevent timing attacks.
	if !hmac.Equal([]byte(s.sign(encoded)), []byte(sig)) {
		return "", false
	}

	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return "", false
	}
	var p sessionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", false
	}

	if time.Now().Unix() > p.ExpiresAt {
		return "", false
	}
	return p.AccessKey, true
}

// Clear deletes the session cookie.
func (s *Session) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Value:   "",
		Path:    "/dirio/ui/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})
}

func (s *Session) sign(data string) string {
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// SetFlash sets a signed flash cookie with the given message and type.
func (s *Session) SetFlash(w http.ResponseWriter, message, msgType string) {
	data := FlashData{Message: message, Type: msgType}
	raw, _ := json.Marshal(data)
	encoded := hex.EncodeToString(raw)
	cookieValue := encoded + "." + s.sign(encoded)

	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    cookieValue,
		Path:     "/dirio/ui/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// GetFlash reads, verifies, and clears the flash cookie.
func (s *Session) GetFlash(w http.ResponseWriter, r *http.Request) (FlashData, bool) {
	cookie, err := r.Cookie(flashCookieName)
	if err != nil {
		return FlashData{}, false
	}

	// Clear the flash cookie immediately.
	http.SetCookie(w, &http.Cookie{
		Name:    flashCookieName,
		Value:   "",
		Path:    "/dirio/ui/",
		MaxAge:  -1,
		Expires: time.Unix(0, 0),
	})

	idx := strings.LastIndex(cookie.Value, ".")
	if idx < 0 {
		return FlashData{}, false
	}
	encoded, sig := cookie.Value[:idx], cookie.Value[idx+1:]

	if !hmac.Equal([]byte(s.sign(encoded)), []byte(sig)) {
		return FlashData{}, false
	}

	raw, err := hex.DecodeString(encoded)
	if err != nil {
		return FlashData{}, false
	}
	var data FlashData
	if err := json.Unmarshal(raw, &data); err != nil {
		return FlashData{}, false
	}

	return data, true
}
