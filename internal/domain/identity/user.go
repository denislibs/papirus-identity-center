package identity

import "time"

// User is a platform account (identity), independent of any workspace/product.
type User struct {
	ID            string
	Email         string
	EmailVerified bool
	PasswordHash  string
	Name          string
	AvatarURL     string
	Locale        string
	Timezone      string
	CreatedAt     time.Time
}
