package tui

import (
	"encoding/json"

	"github.com/jhump/protoreflect/dynamic"
)

type moduleWrap struct {
	Module string          `json:"module"`
	Data   json.RawMessage `json:"data"`
}

func (ui *TUI) jsonFunc() func(mod string, msg *dynamic.Message) ([]byte, error) {
	if ui.prettyPrintOutput {
		if ui.moduleWrapOutput {
			return func(mod string, msg *dynamic.Message) (cnt []byte, err error) {
				cnt, err = msg.MarshalJSONIndent()
				if err != nil {
					return
				}
				return json.MarshalIndent(moduleWrap{Module: mod, Data: json.RawMessage(cnt)}, "", "  ")
			}
		} else {
			return func(mod string, msg *dynamic.Message) ([]byte, error) {
				return msg.MarshalJSONIndent()
			}
		}
	} else {
		if ui.moduleWrapOutput {
			return func(mod string, msg *dynamic.Message) (cnt []byte, err error) {
				cnt, err = msg.MarshalJSON()
				if err != nil {
					return
				}
				return json.Marshal(moduleWrap{Module: mod, Data: json.RawMessage(cnt)})
			}
		} else {
			return func(mod string, msg *dynamic.Message) (cnt []byte, err error) {
				return msg.MarshalJSON()
			}
		}
	}

}
