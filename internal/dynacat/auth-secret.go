package dynacat

import (
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"log/slog"
	"os"
)

// generatedSecretKeyName is where DashDen keeps the sign-in signing key it
// makes for you, in config/.env.
const generatedSecretKeyName = "DASHDEN_AUTH_SECRET"

// saveGeneratedSecret writes a generated value to config/.env. serveApp sets
// it once the config path is known; tests leave it nil.
var saveGeneratedSecret func(name, value string) error

// resolveAuthSecretKey turns auth.secret-key into signing key bytes.
//
//   - Left out: DASHDEN_AUTH_SECRET is used, and if that isn't set either a
//     key is made and saved to config/.env, so sign-ins survive restarts.
//   - 64 bytes in base64, as upstream Dynacat expects: used as is.
//   - Any other text of 16 or more characters: stretched into a key.
func resolveAuthSecretKey(configured string) ([]byte, error) {
	if configured == "" {
		configured = os.Getenv(generatedSecretKeyName)
	}

	if configured == "" {
		key, err := makeAuthSecretKey(AUTH_SECRET_KEY_LENGTH)
		if err != nil {
			return nil, err
		}
		os.Setenv(generatedSecretKeyName, key)
		if saveGeneratedSecret != nil {
			if err := saveGeneratedSecret(generatedSecretKeyName, key); err != nil {
				slog.Warn("Made a sign-in key but couldn't save it to config/.env, so people will be signed out when DashDen restarts", "error", err)
			} else {
				slog.Info("Made a sign-in key and saved it to config/.env as " + generatedSecretKeyName)
			}
		}
		configured = key
	}

	if b, err := base64.StdEncoding.DecodeString(configured); err == nil && len(b) == AUTH_SECRET_KEY_LENGTH {
		return b, nil
	}
	if len(configured) < 16 {
		return nil, errors.New("auth.secret-key is too short. Remove the secret-key line and DashDen will make and save one for you")
	}
	sum := sha512.Sum512([]byte("dashden-auth-secret\x00" + configured))
	return sum[:AUTH_SECRET_KEY_LENGTH], nil
}
