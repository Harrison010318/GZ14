package db

import "errors"

var (
	ErrAccountNotFound = errors.New("account not found")
	ErrWrongPassword   = errors.New("wrong password")
	ErrAccountExist    = errors.New("account already exists")
	ErrRoleNameExist   = errors.New("role name already exists")
	ErrRoleFull        = errors.New("role count limit reached")
	ErrSessionInvalid  = errors.New("session invalid")
	ErrSessionExpired  = errors.New("session expired")
)
