package marshaller

import (
	"fmt"
	"io"

	pbstore "github.com/streamingfast/substreams/storage/store/marshaller/pb"
)

type VTproto struct{}

func (p *VTproto) Unmarshal(in []byte) (*StoreData, uint64, error) {
	stateData := &pbstore.StoreData{}
	dataSize, err := unmarshalVT(stateData, in)
	if err != nil {
		return nil, 0, fmt.Errorf("unmarshal store: %w", err)
	}
	return &StoreData{
		Kv:             stateData.GetKv(),
		DeletePrefixes: stateData.GetDeletePrefixes(),
	}, dataSize, nil
}

func (p *VTproto) Marshal(data *StoreData) ([]byte, error) {
	stateData := &pbstore.StoreData{
		Kv:             data.Kv,
		DeletePrefixes: data.DeletePrefixes,
	}

	return stateData.MarshalVT()
}

// The function `func (m *StoreData) UnmarshalVT(dAtA []byte) error` that is generated
// by the vtprotobuf protobuf plugin is ok, but we can greatly improve the allocation and
// speed with a few optimizations. This function is a 98% copy of the function in
// ./pb/store_vtproto.pb.go
// we've added byte counter too
func unmarshalVT(m *pbstore.StoreData, dAtA []byte) (dataSize uint64, err error) {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return 0, fmt.Errorf("proto: StoreData: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return 0, fmt.Errorf("proto: StoreData: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field Kv", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			if postIndex > l {
				return 0, io.ErrUnexpectedEOF
			}
			if m.Kv == nil {
				m.Kv = make(map[string][]byte)
			}
			var mapkey string
			var mapvalue []byte
			for iNdEx < postIndex {
				entryPreIndex := iNdEx
				var wire uint64
				for shift := uint(0); ; shift += 7 {
					if shift >= 64 {
						return 0, pbstore.ErrIntOverflow
					}
					if iNdEx >= l {
						return 0, io.ErrUnexpectedEOF
					}
					b := dAtA[iNdEx]
					iNdEx++
					wire |= uint64(b&0x7F) << shift
					if b < 0x80 {
						break
					}
				}
				fieldNum := int32(wire >> 3)
				if fieldNum == 1 {
					var stringLenmapkey uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						stringLenmapkey |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intStringLenmapkey := int(stringLenmapkey)
					if intStringLenmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postStringIndexmapkey := iNdEx + intStringLenmapkey
					if postStringIndexmapkey < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postStringIndexmapkey > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					//mapkey = string(dAtA[iNdEx:postStringIndexmapkey])
					mapkey = unsafeGetString(dAtA[iNdEx:postStringIndexmapkey])

					iNdEx = postStringIndexmapkey
				} else if fieldNum == 2 {
					var mapbyteLen uint64
					for shift := uint(0); ; shift += 7 {
						if shift >= 64 {
							return 0, pbstore.ErrIntOverflow
						}
						if iNdEx >= l {
							return 0, io.ErrUnexpectedEOF
						}
						b := dAtA[iNdEx]
						iNdEx++
						mapbyteLen |= uint64(b&0x7F) << shift
						if b < 0x80 {
							break
						}
					}
					intMapbyteLen := int(mapbyteLen)
					if intMapbyteLen < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					postbytesIndex := iNdEx + intMapbyteLen
					if postbytesIndex < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if postbytesIndex > l {
						return 0, io.ErrUnexpectedEOF
					}

					// @julien do not waste time allocating here
					//mapvalue = make([]byte, mapbyteLen)
					//copy(mapvalue, dAtA[iNdEx:postbytesIndex])
					mapvalue = dAtA[iNdEx:postbytesIndex]
					iNdEx = postbytesIndex

				} else {
					iNdEx = entryPreIndex
					skippy, err := skip(dAtA[iNdEx:])
					if err != nil {
						return 0, err
					}
					if (skippy < 0) || (iNdEx+skippy) < 0 {
						return 0, pbstore.ErrInvalidLength
					}
					if (iNdEx + skippy) > postIndex {
						return 0, io.ErrUnexpectedEOF
					}
					iNdEx += skippy
				}
			}
			m.Kv[mapkey] = mapvalue
			dataSize += uint64(len(mapkey) + len(mapvalue))
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return 0, fmt.Errorf("proto: wrong wireType = %d for field DeletePrefixes", wireType)
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			if postIndex > l {
				return 0, io.ErrUnexpectedEOF
			}

			// @julien do not waste time allocating here
			//m.DeletePrefixes = append(m.DeletePrefixes, string(dAtA[iNdEx:postIndex]))
			m.DeletePrefixes = append(m.DeletePrefixes, unsafeGetString(dAtA[iNdEx:postIndex]))
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skip(dAtA[iNdEx:])
			if err != nil {
				return 0, err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			if (iNdEx + skippy) > l {
				return 0, io.ErrUnexpectedEOF
			}
			//m.unknownFields = append(m.unknownFields, dAtA[iNdEx:iNdEx+skippy]...)
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return 0, io.ErrUnexpectedEOF
	}
	return
}

func skip(dAtA []byte) (n int, err error) {
	l := len(dAtA)
	iNdEx := 0
	depth := 0
	for iNdEx < l {
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return 0, pbstore.ErrIntOverflow
			}
			if iNdEx >= l {
				return 0, io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= (uint64(b) & 0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		wireType := int(wire & 0x7)
		switch wireType {
		case 0:
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				iNdEx++
				if dAtA[iNdEx-1] < 0x80 {
					break
				}
			}
		case 1:
			iNdEx += 8
		case 2:
			var length int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return 0, pbstore.ErrIntOverflow
				}
				if iNdEx >= l {
					return 0, io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				length |= (int(b) & 0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if length < 0 {
				return 0, pbstore.ErrInvalidLength
			}
			iNdEx += length
		case 3:
			depth++
		case 4:
			if depth == 0 {
				return 0, pbstore.ErrUnexpectedEndOfGroup
			}
			depth--
		case 5:
			iNdEx += 4
		default:
			return 0, fmt.Errorf("proto: illegal wireType %d", wireType)
		}
		if iNdEx < 0 {
			return 0, pbstore.ErrInvalidLength
		}
		if depth == 0 {
			return iNdEx, nil
		}
	}
	return 0, io.ErrUnexpectedEOF
}
