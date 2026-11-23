package openhandle

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestVerifyWebhookRejectsJSONThatIsNotAnObject(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"
	const timestamp = "1795521600"
	for _, body := range []string{"null", "[]", "1", `"event"`, " \n[{}]"} {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp + "." + body))
		header := http.Header{}
		header.Set("X-Openhandle-Timestamp", timestamp)
		header.Set("X-Openhandle-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))

		_, err := VerifyWebhook([]byte(body), header, secret, &VerifyWebhookOptions{Now: time.Unix(1795521600, 0)})

		var verificationError *WebhookVerificationError
		if !errors.As(err, &verificationError) || verificationError.Code != WebhookInvalidPayload {
			t.Fatalf("body %q: error = %T %v, want invalid_payload", body, err, err)
		}
	}
}
