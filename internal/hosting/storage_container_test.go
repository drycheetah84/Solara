package hosting

import (
	"testing"

	"solara/internal/stj"
)

// Expected bytes captured from the original Hosting assembly (.NET 6 STJ):
// derived-class members serialize before inherited ones.
func TestContainerModelsMatchOracle(t *testing.T) {
	n, tk, m := "n", "t", "m"
	s := ContainerInformationSensitive{Temporary: true, Ticket: &tk, ContainerInformation: ContainerInformation{Name: &n}}
	want1 := `{"Temporary":true,"ExpiresAt":0,"Ticket":"t","Name":"n","Description":null,"Owner":0,"CreatedAt":0,"UpdatedAt":0,"Readable":false,"Writeable":false,"Size":0,"FileCount":0}`
	if got := stj.MarshalString(s); got != want1 {
		t.Errorf("\n got %s\nwant %s", got, want1)
	}
	r := ContainerMessageResultOf[ContainerInformationSensitive]{Value: s, ContainerMessageResult: ContainerMessageResult{Success: true, Message: &m}}
	want2 := `{"Value":` + want1 + `,"Success":true,"Message":"m"}`
	if got := stj.MarshalString(r); got != want2 {
		t.Errorf("\n got %s\nwant %s", got, want2)
	}
}
