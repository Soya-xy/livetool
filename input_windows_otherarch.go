//go:build windows && !amd64

package main

import (
	"context"
	"errors"
)

func runSystemInput(context.Context, Action) error {
	return errors.New("系统键鼠动作目前仅支持 Windows amd64 版本")
}
