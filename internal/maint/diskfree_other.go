//go:build !unix

package maint

import "errors"

// DiskFree is only implemented on Unix (the rockola runs Linux).
func DiskFree(path string) (free, total uint64, err error) {
	return 0, 0, errors.New("espacio libre: no disponible en este sistema")
}
