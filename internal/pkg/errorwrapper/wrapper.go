package errorwrapper

import "fmt"

func Wrap(domainErr error, err error) error {
	if err == nil {
		return domainErr
	}

	return fmt.Errorf("%w: %v", domainErr, err)
}
