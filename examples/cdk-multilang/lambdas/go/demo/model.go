package demo

import "os"

type DemoItem struct {
	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	SK     string `theorydb:"sk,attr:SK" json:"SK"`
	Value  string `theorydb:"attr:value,omitempty" json:"value,omitempty"`
	Lang   string `theorydb:"attr:lang,omitempty" json:"lang,omitempty"`
	Secret string `theorydb:"encrypted,attr:secret,omitempty" json:"secret,omitempty"`
}

func (DemoItem) TableName() string {
	return os.Getenv("TABLE_NAME")
}
