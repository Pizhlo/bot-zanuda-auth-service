package auth

import (
	"auth-service/internal/service/id"
	"context"
	"errors"

	"github.com/sirupsen/logrus"
)

const masterKey = "leader"

func (s *Service) checkMasterKey(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"service": "auth",
	}).Info("checking master key")

	key, err := s.redisClient.Get(ctx, masterKey)
	if err != nil {
		return err
	}

	if key == "" {
		return errors.New("key is empty")
	}

	return nil
}

func (s *Service) setMasterKey(ctx context.Context) error {
	logrus.WithFields(logrus.Fields{
		"service": "auth",
	}).Info("setting master key")

	id, err := id.Generate(10)
	if err != nil {
		return err
	}

	ok, err := s.redisClient.SetWithLocking(ctx, masterKey, id, 0)
	if err != nil {
		return err
	}

	if !ok {
		return errors.New("failed to set master key")
	}

	s.isMaster = true

	logrus.WithFields(logrus.Fields{
		"service": "auth",
		"id":      id,
	}).Info("master key set")

	return nil
}
