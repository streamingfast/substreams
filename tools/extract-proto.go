package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/streamingfast/cli"
	"github.com/streamingfast/cli/sflags"
	"github.com/streamingfast/substreams"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/types/descriptorpb"
)

// schemaProtoPath is where the SQL sink's annotations live, and the import path a proto
// has to use for them. The file is written alongside so the override parses without a
// protobuf toolchain: imports resolve relative to the working directory.
const schemaProtoPath = "sf/substreams/sink/sql/schema/v1/schema.proto"

// schemaProtoPackage is the proto package that file declares, which is what qualifies the
// extensions: `option (schema.table)`, not the file's directory path.
const schemaProtoPackage = "schema"

var extractProtoCmd = &cobra.Command{
	Use:   "extract-proto [<manifest> [<module>]]",
	Short: "Write a module's output protobuf definition to disk",
	Long: cli.Dedent(`
		Write the protobuf file defining a module's output message, as the package bundles
		it, so it can be edited and fed back to a sink.

		With --sql, the file comes annotated for 'substreams sink postgres' in from-proto
		mode: the schema annotations are imported, every message carries a commented-out
		table option and every field a commented-out column option, and the annotations
		file itself is written next to it so the result parses as-is. Uncomment what the
		schema should declare — which field is the primary key, which are unique, which
		reference another table — and point the sink at the result:

		    substreams tools extract-proto --sql substreams.yaml map_events
		    substreams sink postgres setup <manifest> --proto-file-override=./map_events.proto --dsn=...

		Without annotations a from-proto sink derives tables from the message structure
		alone: no primary keys, no unique constraints, no foreign keys. Only the index on
		_block_number_ is created, that one being the sink's own rather than the schema's.
	`),
	Args: cobra.RangeArgs(0, 2),
	RunE: runExtractProtoE,
}

func init() {
	extractProtoCmd.Flags().Bool("sql", false, "Annotate the output for the SQL sink and write the annotations file beside it")
	extractProtoCmd.Flags().String("output-dir", ".", "Directory the files are written to")

	Cmd.AddCommand(extractProtoCmd)
}

func runExtractProtoE(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true

	manifestPath := ""
	if len(args) > 0 {
		manifestPath = args[0]
	}
	moduleName := ""
	if len(args) > 1 {
		moduleName = args[1]
	}

	reader, err := manifest.NewReader(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}

	pkgBundle, err := reader.Read()
	if err != nil {
		return fmt.Errorf("reading manifest %q: %w", manifestPath, err)
	}
	pkg := pkgBundle.Package

	module, err := resolveOutputModule(pkg, moduleName)
	if err != nil {
		return err
	}

	outputType := strings.TrimPrefix(module.Output.GetType(), "proto:")
	file, err := fileDefining(pkg, outputType)
	if err != nil {
		return err
	}

	outputDir := sflags.MustGetString(cmd, "output-dir")
	annotate := sflags.MustGetBool(cmd, "sql")

	rendered := renderProtoFile(file, outputType, annotate)

	target := filepath.Join(outputDir, filepath.Base(file.GetName()))
	if err := writeFile(target, rendered); err != nil {
		return err
	}
	fmt.Printf("Wrote %s (message %s)\n", target, outputType)

	if annotate {
		// The annotations file has to be on disk for the import to resolve: the override
		// is parsed with imports looked up from the working directory. Writing the copy
		// this binary was built with removes the one step most likely to go wrong.
		schemaTarget := filepath.Join(outputDir, schemaProtoPath)
		if err := writeFile(schemaTarget, substreams.SQLSchemaProto); err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", schemaTarget)

		fmt.Printf("\nUncomment the options that describe the schema, then:\n"+
			"  substreams sink postgres setup <manifest> --proto-file-override=%s --dsn=...\n", target)
	}

	return nil
}

// resolveOutputModule picks the module to extract, inferring it when there is only one map
// module to infer.
func resolveOutputModule(pkg *pbsubstreams.Package, name string) (*pbsubstreams.Module, error) {
	var candidates []*pbsubstreams.Module
	for _, module := range pkg.Modules.Modules {
		if module.GetKindMap() != nil {
			candidates = append(candidates, module)
		}
	}

	if name != "" {
		for _, module := range candidates {
			if module.Name == name {
				return module, nil
			}
		}

		return nil, fmt.Errorf("no map module named %q in this package", name)
	}

	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("this package has no map module to extract an output type from")
	case 1:
		return candidates[0], nil
	}

	names := make([]string, len(candidates))
	for i, module := range candidates {
		names[i] = module.Name
	}
	sort.Strings(names)

	return nil, fmt.Errorf("this package has more than one map module, name the one to extract: %s", strings.Join(names, ", "))
}

func findFile(pkg *pbsubstreams.Package, name string) *descriptorpb.FileDescriptorProto {
	for _, file := range pkg.ProtoFiles {
		if file.GetName() == name {
			return file
		}
	}

	return nil
}

// fileDefining finds the bundled file that declares the output message.
func fileDefining(pkg *pbsubstreams.Package, outputType string) (*descriptorpb.FileDescriptorProto, error) {
	for _, file := range pkg.ProtoFiles {
		for _, message := range file.MessageType {
			if file.GetPackage()+"."+message.GetName() == outputType {
				return file, nil
			}
		}
	}

	return nil, fmt.Errorf("the package does not bundle a definition for %q", outputType)
}

func writeFile(path string, content string) error {
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", directory, err)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// renderProtoFile turns a bundled descriptor back into readable proto source.
//
// The descriptor is what the package carries — comments and formatting are long gone — so
// this reconstructs a file rather than recovering the original. With annotate, every
// message and field gets the SQL sink's options commented out beside it, which is the part
// that saves the reader looking them up.
func renderProtoFile(file *descriptorpb.FileDescriptorProto, outputType string, annotate bool) string {
	var out strings.Builder

	out.WriteString("syntax = \"" + file.GetSyntax() + "\";\n\n")
	out.WriteString("package " + file.GetPackage() + ";\n\n")

	for _, dependency := range file.Dependency {
		out.WriteString("import \"" + dependency + "\";\n")
	}
	if annotate && !hasDependency(file, schemaProtoPath) {
		out.WriteString("import \"" + schemaProtoPath + "\";\n")
	}
	if len(file.Dependency) > 0 || annotate {
		out.WriteString("\n")
	}

	for _, message := range file.MessageType {
		renderMessage(&out, file, message, outputType, annotate, "")
	}

	for _, enum := range file.EnumType {
		out.WriteString("enum " + enum.GetName() + " {\n")
		for _, value := range enum.Value {
			fmt.Fprintf(&out, "  %s = %d;\n", value.GetName(), value.GetNumber())
		}
		out.WriteString("}\n\n")
	}

	return out.String()
}

func hasDependency(file *descriptorpb.FileDescriptorProto, name string) bool {
	for _, dependency := range file.Dependency {
		if dependency == name {
			return true
		}
	}

	return false
}

func renderMessage(out *strings.Builder, file *descriptorpb.FileDescriptorProto, message *descriptorpb.DescriptorProto, outputType string, annotate bool, indent string) {
	fullName := file.GetPackage() + "." + message.GetName()

	out.WriteString(indent + "message " + message.GetName() + " {\n")

	if annotate {
		if fullName == outputType {
			out.WriteString(indent + "  // This is the module's output message. The sink walks it into tables: every\n")
			out.WriteString(indent + "  // nested message becomes a table of its own, repeated ones a table per element.\n")
		}
		out.WriteString(indent + "  // option (" + schemaProtoPackage + ".table) = { name: \"" + strings.ToLower(message.GetName()) + "\" };\n\n")
	}

	for _, field := range message.Field {
		if annotate {
			fmt.Fprintf(out, "%s  // [("+schemaProtoPackage+".field) = { primary_key: true }]  // one per message\n", indent)
			fmt.Fprintf(out, "%s  // [("+schemaProtoPackage+".field) = { unique: true }]\n", indent)
			fmt.Fprintf(out, "%s  // [("+schemaProtoPackage+".field) = { foreign_key: \"other_table.column\" }]\n", indent)
		}

		fmt.Fprintf(out, "%s  %s%s %s = %d;\n", indent, labelOf(field), typeOf(field), field.GetName(), field.GetNumber())

		if annotate {
			out.WriteString("\n")
		}
	}

	for _, nested := range message.NestedType {
		if nested.GetOptions().GetMapEntry() {
			continue
		}
		out.WriteString("\n")
		renderMessage(out, file, nested, outputType, annotate, indent+"  ")
	}

	out.WriteString(indent + "}\n\n")
}

func labelOf(field *descriptorpb.FieldDescriptorProto) string {
	if field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return "repeated "
	}
	if field.GetProto3Optional() {
		return "optional "
	}

	return ""
}

func typeOf(field *descriptorpb.FieldDescriptorProto) string {
	if field.TypeName != nil {
		return strings.TrimPrefix(field.GetTypeName(), ".")
	}

	name := strings.ToLower(strings.TrimPrefix(field.GetType().String(), "TYPE_"))
	if name == "" {
		return "bytes"
	}

	return name
}
