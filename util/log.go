package util

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"strings"
)

type FormatterType uint8

const (
	DEFAULT FormatterType = iota
	PLAIN
)

var (
	formatterNameToType = map[string]FormatterType{
		"default": DEFAULT,
		"plain":   PLAIN,
	}
	formatterTypeToName = map[FormatterType]string{
		DEFAULT: "default",
		PLAIN:   "plain",
	}
)

func ParseFormatterType(s string) (FormatterType, error) {
	s = strings.ToLower(s)
	formatterType, ok := formatterNameToType[s]
	if !ok {
		return formatterType, fmt.Errorf("invalid formatter type: %s")
	}
	return formatterType, nil
}

func (f FormatterType) String() string {
	return formatterTypeToName[f]
}

func (f FormatterType) Apply() {
	switch f {
	case DEFAULT:
	case PLAIN:
		logrus.SetFormatter(PlainFormatter{})
	default:
		panic(fmt.Sprintf("invalid formatter type %v", f))
	}
}

type FormatterFlag FormatterType

func (f *FormatterFlag) String() string {
	return FormatterType(*f).String()
}

func (f *FormatterFlag) Set(value string) error {
	formatterType, err := ParseFormatterType(value)
	if err != nil {
		return err
	}

	*f = FormatterFlag(formatterType)
	return nil
}

type PlainFormatter struct{}

func (_ PlainFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("%s\n", entry.Message)), nil
}

type LevelFlag logrus.Level

func (l *LevelFlag) String() string {
	return logrus.Level(*l).String()
}

func (l *LevelFlag) Set(value string) error {
	lvl, err := logrus.ParseLevel(value)
	if err != nil {
		return err
	}

	*l = LevelFlag(lvl)
	return nil
}

func (l *LevelFlag) Level() logrus.Level {
	return logrus.Level(*l)
}
