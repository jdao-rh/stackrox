package printer

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/stackrox/rox/pkg/errorhelpers"
)

// TabularPrinterFactory holds all configuration options of tabular printers, specifically CSVPrinter and TablePrinter
// It is an implementation of CustomPrinterFactory and acts as a factory for tabular printers
type TabularPrinterFactory struct {
	// Merge only applies to the "table" format and merges certain cells within the output
	Merge                 bool
	Headers               []string
	RowJSONPathExpression string
	NoHeader              bool
	// HeaderAsComment only applies to the "csv" format and prints headers as comment lines in the CSV output
	HeaderAsComment bool
}

// NewTabularPrinterFactory creates new TabularPrinterFactory with the injected default values
func NewTabularPrinterFactory(merge bool, headers []string, rowJSONPathExpression string, noHeader, headerAsComment bool) *TabularPrinterFactory {
	return &TabularPrinterFactory{
		Merge:                 merge,
		Headers:               headers,
		RowJSONPathExpression: rowJSONPathExpression,
		NoHeader:              noHeader,
		HeaderAsComment:       headerAsComment,
	}
}

// AddFlags will add all tabular printer specific flags to the cobra.Command
func (t *TabularPrinterFactory) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().BoolVar(&t.Merge, "merge-output", t.Merge, "Merge duplicate cells in prettified tabular output")
	cmd.PersistentFlags().StringSliceVar(&t.Headers, "headers", t.Headers, "Headers to print in tabular output")
	cmd.PersistentFlags().StringVar(&t.RowJSONPathExpression, "row-jsonpath-expressions", t.RowJSONPathExpression,
		"JSON Path expression to create a row from the JSON object. This leverages gJSON (https://github.com/tidwall/gjson)."+
			" NOTE: The amount of expressions within the multi-path has to match the amount of provided headers.")
	cmd.PersistentFlags().BoolVar(&t.NoHeader, "no-header", t.NoHeader, "Print no headers for tabular output")
	cmd.PersistentFlags().BoolVar(&t.HeaderAsComment, "headers-as-comments", t.HeaderAsComment, "Print headers "+
		"as comments in CSV tabular output")
}

// SupportedFormats returns the supported printer format that can be created by TabularPrinterFactory
func (t *TabularPrinterFactory) SupportedFormats() []string {
	return []string{"table", "csv"}
}

// CreatePrinter creates a tabular printer from the options set. If the format is unsupported, or it is not possible
// to create an ObjectPrinter with the current configuration it will return an error
// A tabular printer expects a JSON Object and JSON Path expression that is compatible
// with GJSON (https://github.com/tidwall/gjson).
// When printing, the tabular printers will take the given JSON object, apply a row expression via a gjson
// multi path expression to retrieve the data from the JSON object and print the result in tabular format.
// The JSON Object itself MUST be passable to json.Marshal, so it CAN NOT be a direct JSON input.
// For the structure of the JSON object, it is preferred to have arrays of structs instead of
// array of elements, since structs will provide default values if the field is missing.
// The gjson expression syntax (https://github.com/tidwall/gjson/blob/master/SYNTAX.md) offers more complex
// and advanced scenarios, if you require them and the below example is not sufficient.
// The following example illustrates a JSON compatible structure and its gjson multi path expression
// JSON structure:
// type data struct {
//		Infos 	[]info `json:"infos"`
//		Name 	string `json:"name"`
// }
// type info struct {
//		info 	string `json:"info"`
//		topic 	string `json:"topic"`
// }
// Data:
// data := &data{Name: "example", Infos: []info{
//										{info: "info1", topic: "topic1"},
//										{info: "info2", topic: "topic2"},
//										{info: "info3", topic: "topic3"},
//										}
// gjson multi path expression: "{name,infos.#.info,infos.#.topic}"
// 	- bundle multiple gjson expression surrounded by "{}" to form a multi path expression
// 	- specify "#" to visit each element in the array
// 	- each expression in the multi path expression is correlated with the given header(s)!
//
// headers := []string{"name", "info", "topic"}
//
// This would result in the following rows for the tabular printers
// | name	 | info  | topic  |
// | example | info1 | topic1 |
// | example | info2 | topic2 |
// | example | info3 | topic3 |
func (t *TabularPrinterFactory) CreatePrinter(format string) (ObjectPrinter, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}
	switch strings.ToLower(format) {
	case "table":
		return newTablePrinter(t.Headers, t.RowJSONPathExpression, t.Merge, t.NoHeader), nil
	case "csv":
		return newCSVPrinter(t.Headers, t.RowJSONPathExpression, t.NoHeader, t.HeaderAsComment), nil
	default:
		return nil, errorhelpers.NewErrInvalidArgs(fmt.Sprintf("invalid output format used for "+
			"Tabular Printer: %q", format))
	}
}

// Validate verifies whether the current configuration can be used to create an ObjectPrinter. It will return an error
// if it is not possible
func (t *TabularPrinterFactory) validate() error {
	// verify that the GJSON multi path expression matches the amount of headers.
	// Example: multi-path expression: {some.expression,another.expression}
	amountJSONPathExpressions := 0
	if t.RowJSONPathExpression != "" {
		amountJSONPathExpressions = len(strings.Split(t.RowJSONPathExpression, ","))
	}

	if len(t.Headers) != amountJSONPathExpressions {
		return errorhelpers.NewErrInvalidArgs("different number of headers and JSON Path expressions " +
			"specified. Make sure you specify the same amount of arguments for both")
	}

	if t.NoHeader && t.HeaderAsComment {
		return errorhelpers.NewErrInvalidArgs("cannot specify both --no-header as well as " +
			"--headers-as-comment flags. Choose only one of them")
	}
	return nil
}
