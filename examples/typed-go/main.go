// Package main shows the additive Go typed API surface without performing
// network I/O. The example accepts an existing TableTheory DB handle from the
// application's normal initialization path (for example LambdaInit or New).
package main

import (
	"github.com/theory-cloud/tabletheory/v2"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	"github.com/theory-cloud/tabletheory/v2/pkg/query"
)

type User struct {
	PK      string `theorydb:"pk" json:"PK"`
	SK      string `theorydb:"sk" json:"SK"`
	Name    string `json:"name"`
	Version int64  `theorydb:"version" json:"version"`
}

func typedFlow(db core.DB) error {
	users := tabletheory.ModelOf[User](db)

	user := User{PK: "TENANT#demo", SK: "USER#ada", Name: "Ada"}
	if err := users.Create(&user); err != nil {
		return err
	}

	got, err := users.Get(users.Key("TENANT#demo", "USER#ada"))
	if err != nil {
		return err
	}

	got.Name = "Ada Lovelace"
	if err := users.Update(&got, "Name"); err != nil {
		return err
	}

	_, err = users.Query().Where("SK", query.OpBeginsWith, "USER#").First()
	return err
}

func main() {}
