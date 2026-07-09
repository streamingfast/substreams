package schema

import (
	"fmt"

	schema "github.com/streamingfast/substreams/pb/sf/substreams/sink/sql/schema/v1"
	"github.com/streamingfast/substreams/sink/sql/proto"
	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Schema struct {
	Name                  string
	TableRegistry         map[string]*Table
	logger                *zap.Logger
	rootMessageDescriptor protoreflect.MessageDescriptor
	withProtoOption       bool
}

func NewSchema(name string, rootMessageDescriptor protoreflect.MessageDescriptor, withProtoOption bool, logger *zap.Logger) (*Schema, error) {
	logger.Info("creating schema", zap.String("name", name), zap.String("root_message_descriptor", string(rootMessageDescriptor.Name())), zap.Bool("with_proto_option", withProtoOption))
	s := &Schema{
		Name:                  name,
		TableRegistry:         make(map[string]*Table),
		logger:                logger,
		rootMessageDescriptor: rootMessageDescriptor,
		withProtoOption:       withProtoOption,
	}

	err := s.init(rootMessageDescriptor)
	if err != nil {
		return nil, fmt.Errorf("initializing schema: %w", err)
	}
	return s, nil
}

func (s *Schema) ChangeName(name string) error {
	s.Name = name
	s.TableRegistry = make(map[string]*Table)
	err := s.init(s.rootMessageDescriptor)
	if err != nil {
		return fmt.Errorf("changing schema name: %w", err)
	}

	return nil
}

func (s *Schema) init(rootMessageDescriptor protoreflect.MessageDescriptor) error {
	s.logger.Info("initializing schema", zap.String("name", s.Name), zap.String("root_message_descriptor", string(rootMessageDescriptor.Name())))
	err := s.walkMessageDescriptor(rootMessageDescriptor, 0, func(md protoreflect.MessageDescriptor, ordinal int) error {
		s.logger.Debug("creating table message descriptor", zap.String("message_descriptor_name", string(md.Name())), zap.Int("ordinal", ordinal))
		tableInfo := proto.TableInfo(md)
		if tableInfo == nil {
			if s.withProtoOption {
				return nil
			}
			tableInfo = &schema.Table{
				Name:    string(md.Name()),
				ChildOf: nil,
			}
		}
		if _, found := s.TableRegistry[tableInfo.Name]; found {
			return nil
		}
		table, err := NewTable(md, tableInfo, ordinal, 0)
		if err != nil {
			return fmt.Errorf("creating table message descriptor: %w", err)
		}
		if table != nil {
			s.logger.Debug("created table message descriptor", zap.String("message_descriptor_name", string(md.Name())), zap.Int("ordinal", ordinal), zap.String("table_name", table.Name))
			s.TableRegistry[tableInfo.Name] = table
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("walking and creating table message descriptors registry: %q: %w", string(rootMessageDescriptor.Name()), err)
	}

	return nil
}

func (s *Schema) walkMessageDescriptor(md protoreflect.MessageDescriptor, ordinal int, task func(md protoreflect.MessageDescriptor, ordinal int) error) error {
	s.logger.Debug("walking message descriptor", zap.String("message_descriptor_name", string(md.Name())), zap.Int("ordinal", ordinal))
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		s.logger.Debug("walking field", zap.String("field_name", string(field.Name())), zap.String("field_type", field.Kind().String()))
		if field.Kind() == protoreflect.MessageKind {
			err := s.walkMessageDescriptor(field.Message(), ordinal+1, task)
			if err != nil {
				return fmt.Errorf("walking field %q message descriptor: %w", string(field.Name()), err)
			}
		}
	}

	err := task(md, ordinal)
	if err != nil {
		return fmt.Errorf("running task on message descriptor %q: %w", string(md.Name()), err)
	}

	return nil
}

func (s *Schema) String() string {
	return fmt.Sprintf("%s", s.Name)
}
