package security

import (
	"encoding/hex"
	"net/http"
)

type CookieManager struct {
	name   string
	path   string
	maxAge int // seconds; 0 = session cookie
	secure bool
}

type CookieOption func(*CookieManager)

func WithMaxAge(seconds int) CookieOption { return func(m *CookieManager) { m.maxAge = seconds } }
func WithSecure(secure bool) CookieOption { return func(m *CookieManager) { m.secure = secure } }
func WithPath(path string) CookieOption   { return func(m *CookieManager) { m.path = path } }

func NewCookieManager(name string, opts ...CookieOption) *CookieManager {
	m := &CookieManager{name: name, path: "/"}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *CookieManager) WithID(id string) *CookieManager {
	c := *m
	c.name = m.name + "_" + id
	return &c
}

func (m *CookieManager) Set(w http.ResponseWriter, value string) {
	http.SetCookie(w, m.cookie(value, m.maxAge))
}

func (m *CookieManager) Get(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(m.name)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (m *CookieManager) Clear(w http.ResponseWriter) {
	http.SetCookie(w, m.cookie("", -1))
}

func (m *CookieManager) SetTag(w http.ResponseWriter, secret []byte, msg string) {
	m.Set(w, hex.EncodeToString(Sign(secret, msg)))
}

func (m *CookieManager) CheckTag(r *http.Request, secret []byte, msg string) bool {
	value, ok := m.Get(r)
	if !ok {
		return false
	}
	tag, err := hex.DecodeString(value)
	if err != nil {
		return false
	}
	return VerifyTag(secret, msg, tag)
}

func (m *CookieManager) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     m.name,
		Value:    value,
		Path:     m.path,
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.secure,
	}
}
