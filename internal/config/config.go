package config

import "os"

type Config struct {
	Port             string
	DatabaseURL      string
	JWTSecret        string
	FrontendOrigin   string
	TwilioAccountSID string
	TwilioAuthToken  string
	TwilioFromSMS    string
	TwilioFromWA     string
	MPesaBaseURL     string
	MPesaConsumerKey string
	MPesaSecret      string
	MPesaShortCode   string
	UploadDir        string
	PublicBaseURL    string
	BcryptCost       int
	JWTExpiryHours   int
	Env              string
}

func Load() Config {
	return Config{
		Port:             env("PORT", "8080"),
		DatabaseURL:      env("DATABASE_URL", "postgres://rentpulse:rentpulse@localhost:5432/rentpulse?sslmode=disable"),
		JWTSecret:        env("JWT_SECRET", ""),
		FrontendOrigin:   env("FRONTEND_ORIGIN", "http://localhost:5173"),
		TwilioAccountSID: env("TWILIO_ACCOUNT_SID", ""),
		TwilioAuthToken:  env("TWILIO_AUTH_TOKEN", ""),
		TwilioFromSMS:    env("TWILIO_FROM_SMS", ""),
		TwilioFromWA:     env("TWILIO_FROM_WHATSAPP", ""),
		MPesaBaseURL:     env("MPESA_BASE_URL", "https://sandbox.safaricom.co.ke"),
		MPesaConsumerKey: env("MPESA_CONSUMER_KEY", ""),
		MPesaSecret:      env("MPESA_CONSUMER_SECRET", ""),
		MPesaShortCode:   env("MPESA_SHORT_CODE", ""),
		UploadDir:        env("UPLOAD_DIR", "uploads"),
		PublicBaseURL:    env("PUBLIC_BASE_URL", "http://localhost:8080"),
		BcryptCost:       parseInt(env("BCRYPT_COST", "12")),
		JWTExpiryHours:   parseInt(env("JWT_EXPIRY_HOURS", "1")),
		Env:              env("APP_ENV", "development"),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseInt(value string) int {
	n := 0
	for _, c := range value {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}
