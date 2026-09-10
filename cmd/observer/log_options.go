package main

import (
	"errors"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/logobs"
)

type logSourceFlags []string

func (values *logSourceFlags) String() string { return strings.Join(*values, ",") }

func (values *logSourceFlags) Set(value string) error {
	if len(*values) >= logobs.MaxSources || (value != "system" && value != "application") {
		return errors.New("use one fixed log source per flag: system or application")
	}
	for _, existing := range *values {
		if existing == value {
			return errors.New("duplicate log source")
		}
	}
	*values = append(*values, value)
	return nil
}
