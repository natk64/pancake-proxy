package utils

import (
	"context"
	"time"

	"go.uber.org/zap"
)

type AutoRestarter struct {
	Name   string
	Delay  time.Duration
	Logger *zap.Logger
	F      func(ctx context.Context) error
}

// Run will call the function F until the context is cancelled.
func (ar AutoRestarter) Run(ctx context.Context) error {
	for {
		if err := ar.F(ctx); err != nil {
			ar.Logger.Error("Task stopped with error", zap.String("name", ar.Name), zap.Error(err))
		} else {
			ar.Logger.Info("Task stopped", zap.String("name", ar.Name))
		}

		if err := ctx.Err(); err != nil {
			ar.Logger.Info("Task cancelled", zap.String("name", ar.Name))
			return err
		}

		time.Sleep(ar.Delay)
		ar.Logger.Info("Restarting task", zap.String("name", ar.Name))
	}
}
