package dcmgmt

import "errors"

var (
	ErrEmptyConnString   = errors.New("empty sync connect string")
	ErrInvalidConnString = errors.New("invalid sync connect string")
	ErrInvalidServerHost = errors.New("invalid sync server host")
	ErrInvalidServerPort = errors.New("invalid sync server port")
	ErrEmptyIdent        = errors.New("empty ident")
	ErrEmptyID           = errors.New("empty id")
)
