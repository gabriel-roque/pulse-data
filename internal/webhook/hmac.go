package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/pulse-data/pulse/internal/events"
	"strconv"
	"time"
)

func Signature(secret []byte, event events.Event, timestamp time.Time, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(strconv.FormatInt(timestamp.Unix(), 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifySignature(secret []byte, signature string, timestamp time.Time, body []byte, tolerance time.Duration, now time.Time) bool {
	if tolerance > 0 && (now.Sub(timestamp) > tolerance || timestamp.Sub(now) > tolerance) {
		return false
	}
	expected := Signature(secret, events.Event{}, timestamp, body)
	a, err1 := hex.DecodeString(signature)
	b, err2 := hex.DecodeString(expected)
	return err1 == nil && err2 == nil && hmac.Equal(a, b)
}
