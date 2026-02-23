package service

import "errors"

// ErrUserNotMember ошибка о том, что пользователь не состоит в пространстве.
var ErrUserNotMember = errors.New("user is not member")
