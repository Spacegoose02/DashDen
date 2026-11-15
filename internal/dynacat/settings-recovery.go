package dynacat

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// DASHDEN_RECOVERY is a way back in when a settings change locks everyone
// out, for example a sign-in proxy that no longer lets anyone through. Set it
// on the container, open /settings/recover and type the same code: that
// browser can use Settings for an hour, without signing in. Remove the
// variable once things work again.

const (
	recoveryEnvVar     = "DASHDEN_RECOVERY"
	recoveryCookieName = "dashden_recovery"
	recoveryMinLength  = 16
	recoveryLifetime   = time.Hour
)

var recoveryPageTemplate = mustParseTemplate("settings-recover.html", "document.html", "footer.html")

func recoveryCode() string {
	code := strings.TrimSpace(os.Getenv(recoveryEnvVar))
	if len(code) < recoveryMinLength {
		return ""
	}
	return code
}

func warnAboutRecoveryCode() {
	raw := strings.TrimSpace(os.Getenv(recoveryEnvVar))
	if raw == "" {
		return
	}
	if len(raw) < recoveryMinLength {
		slog.Warn(recoveryEnvVar+" is too short and is ignored", "minimum", recoveryMinLength)
		return
	}
	slog.Warn(recoveryEnvVar + " is set: anyone with the code can open Settings at /settings/recover. Remove it once you're back in.")
}

func recoverySignature(code string, issued int64) string {
	mac := hmac.New(sha256.New, []byte(code))
	mac.Write([]byte("dashden-recovery:" + strconv.FormatInt(issued, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func formatRecoveryCookie(code string, issued int64) string {
	return strconv.FormatInt(issued, 10) + "." + recoverySignature(code, issued)
}

// inRecovery reports whether this browser entered the recovery code recently.
func inRecovery(r *http.Request) bool {
	code := recoveryCode()
	if code == "" {
		return false
	}
	cookie, err := r.Cookie(recoveryCookieName)
	if err != nil {
		return false
	}
	issuedText, sig, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return false
	}
	issued, err := strconv.ParseInt(issuedText, 10, 64)
	if err != nil {
		return false
	}
	age := time.Since(time.Unix(issued, 0))
	if age < 0 || age > recoveryLifetime {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(recoverySignature(code, issued)))
}

func (a *application) handleRecoveryPage(w http.ResponseWriter, r *http.Request) {
	code := recoveryCode()
	if code == "" {
		a.handleNotFound(w, r)
		return
	}

	data := struct {
		templateData
		Failed bool
	}{templateData: templateData{App: a}}

	if r.Method == http.MethodPost {
		// Slow down guessing; the code is long, this just makes it pointless.
		time.Sleep(500 * time.Millisecond)
		r.ParseForm()
		given := strings.TrimSpace(r.PostForm.Get("code"))
		if subtle.ConstantTimeCompare([]byte(given), []byte(code)) == 1 {
			issued := time.Now().Unix()
			http.SetCookie(w, &http.Cookie{
				Name:     recoveryCookieName,
				Value:    formatRecoveryCookie(code, issued),
				Path:     a.Config.Server.BaseURL + "/",
				MaxAge:   int(recoveryLifetime.Seconds()),
				HttpOnly: true,
				Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
				SameSite: http.SameSiteStrictMode,
			})
			slog.Warn("Settings opened with the recovery code", "from", remoteAddrWithoutPort(r))
			http.Redirect(w, r, a.Config.Server.BaseURL+"/settings", http.StatusSeeOther)
			return
		}
		slog.Warn("Wrong recovery code entered", "from", remoteAddrWithoutPort(r))
		data.Failed = true
		w.WriteHeader(http.StatusUnauthorized)
	}

	a.populateTemplateRequestData(&data.Request, r, nil)
	w.Header().Set("Cache-Control", "no-store")
	if err := recoveryPageTemplate.Execute(w, data); err != nil {
		slog.Error("Rendering recovery page failed", "error", err)
	}
}
