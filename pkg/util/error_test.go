package util

import (
	"fmt"
	"testing"

	"github.com/microsoft/kiota-abstractions-go/store"
	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
	"github.com/stretchr/testify/assert"
)

func Test_OdataErorr(t *testing.T) {
	t.Run("Test if interpretation of odata errors is correct", func(t *testing.T) {
		message := "gopher digs"
		code := "1337"

		errors := odataerrors.NewMainError()
		errors.SetMessage(&message)
		errors.SetCode(&code)

		err := new(odataerrors.ODataError)
		err.SetBackingStore(store.NewInMemoryBackingStore())
		err.SetErrorEscaped(errors)
		err.Message = "gopher digs"
		err.SetStatusCode(1337)

		result := GetOdataError(err)
		assert.ErrorContains(t, result, fmt.Sprintf("%s: %s", *err.GetErrorEscaped().GetCode(), *err.GetErrorEscaped().GetMessage()))
	})
}
