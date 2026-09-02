package util

import (
	"errors"
	"fmt"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

func GetOdataError(err error) error {
	var odataError *odataerrors.ODataError
	if errors.As(err, &odataError) {
		if terr := odataError.GetErrorEscaped(); terr != nil {
			statusCode := odataError.GetStatusCode()

			return fmt.Errorf("HTTP %d %s: %s: %w", statusCode, *terr.GetCode(), *terr.GetMessage(), err)
		}
	}

	return err
}
