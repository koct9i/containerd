package typeurl

import (
	"encoding/json"
	"path"
	"reflect"
	"strings"
	"sync"

	"github.com/containerd/containerd/errdefs"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
)

const Prefix = "types.containerd.io"

var (
	mu       sync.Mutex
	registry = make(map[reflect.Type]string)
)

// Register a type with the base url of the type
func Register(v interface{}, args ...string) {
	t := tryDereference(v)
	mu.Lock()
	defer mu.Unlock()
	if _, ok := registry[t]; ok {
		panic(errdefs.ErrAlreadyExists)
	}
	registry[t] = path.Join(append([]string{Prefix}, args...)...)
}

// TypeURL returns the type url for a registred type
func TypeURL(v interface{}) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	u, ok := registry[tryDereference(v)]
	if !ok {
		// fallback to the proto registry if it is a proto message
		pb, ok := v.(proto.Message)
		if !ok {
			return "", errdefs.ErrNotFound
		}
		return path.Join(Prefix, proto.MessageName(pb)), nil
	}
	return u, nil
}

func Is(any *types.Any, v interface{}) bool {
	url, err := TypeURL(v)
	if err != nil {
		return false
	}
	return any.TypeUrl == url
}

func MarshalAny(v interface{}) (*types.Any, error) {
	var (
		err  error
		data []byte
	)
	url, err := TypeURL(v)
	if err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case proto.Message:
		data, err = proto.Marshal(t)
	default:
		data, err = json.Marshal(v)
	}
	if err != nil {
		return nil, err
	}
	return &types.Any{
		TypeUrl: url,
		Value:   data,
	}, nil
}

func UnmarshalAny(any *types.Any) (interface{}, error) {
	t, err := getTypeByUrl(any.TypeUrl)
	if err != nil {
		return nil, err
	}
	v := reflect.New(t.t).Interface()
	if t.isProto {
		err = proto.Unmarshal(any.Value, v.(proto.Message))
	} else {
		err = json.Unmarshal(any.Value, v)
	}
	return v, err
}

type urlType struct {
	t       reflect.Type
	isProto bool
}

func getTypeByUrl(url string) (urlType, error) {
	for t, u := range registry {
		if u == url {
			return urlType{
				t: t,
			}, nil
		}
	}
	// fallback to proto registry
	t := proto.MessageType(strings.TrimPrefix(url, Prefix+"/"))
	if t != nil {
		return urlType{
			// get the underlying Elem because proto returns a pointer to the type
			t:       t.Elem(),
			isProto: true,
		}, nil
	}
	return urlType{}, errdefs.ErrNotFound
}

func tryDereference(v interface{}) reflect.Type {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
