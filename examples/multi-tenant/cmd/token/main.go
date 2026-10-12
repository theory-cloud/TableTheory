// Command token mints a signed HS256 bearer token for the multi-tenant example's
// local development server, so a developer can exercise the authenticated API.
// It is a convenience wrapper around the auth package; the secret it signs with
// must match the JWT_SECRET the server was started with.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/theory-cloud/tabletheory/v4/examples/multi-tenant/auth"
)

func main() {
	secret := flag.String("secret", os.Getenv("JWT_SECRET"), "signing secret (defaults to JWT_SECRET)")
	user := flag.String("user", "", "user ID to place in the token subject")
	org := flag.String("org", "", "organization ID the token is bound to")
	ttl := flag.Int64("ttl", 3600, "token lifetime in seconds")
	flag.Parse()

	if *secret == "" || *user == "" || *org == "" {
		fmt.Fprintln(os.Stderr, "usage: token -secret <secret> -user <user> -org <org> [-ttl seconds]")
		os.Exit(2)
	}

	now := time.Now()
	token, err := auth.Sign(*secret, auth.Claims{
		Subject:   *user,
		OrgID:     *org,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Duration(*ttl) * time.Second).Unix(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to sign token:", err)
		os.Exit(1)
	}

	fmt.Println(token)
}
