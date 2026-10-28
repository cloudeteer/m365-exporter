package util

import (
	"errors"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

func GetOdataError(err error) error {
	if odataError, ok := errors.AsType[*odataerrors.ODataError](err); ok {
		if terr := odataError.GetErrorEscaped(); terr != nil {
			statusCode := odataError.GetStatusCode()

			return fmt.Errorf("HTTP %d %s: %s: %w", statusCode, *terr.GetCode(), *terr.GetMessage(), err)
		}
	}

	return err
}
