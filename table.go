package airtable

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"reflect"
	"strings"
	"time"
)

// Table ...
type Table struct {
	name   string
	client *Client
	record interface{}
}

// Record ...
type Record struct {
	ID          string
	CreatedTime time.Time
}

// Table returns a new table
func (c *Client) Table(name string) Table {
	return Table{
		client: c,
		name:   name,
	}
}

// Get returns information about a resource
func (t *Table) Get(id string, record interface{}) error {
	bytes, err := t.client.Request("GET", t.makePath(id), nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, record)
}

// Update ...
func (t *Table) Update(record interface{}) error {
	id, err := getID(&record)
	if err != nil {
		return err
	}
	body, err := getJSONBody(&record)
	if err != nil {
		return err
	}
	_, err = t.client.RequestWithBody("PATCH", t.makePath(id), Options{}, body)
	if err != nil {
		return err
	}
	return nil
}

// Fields ...
type Fields map[string]interface{}

// NewRecord ...
func NewRecord(container interface{}, data Fields) {
	// iterating over the container fields and applying those keys to
	// the passed in fields would be "safer", but it could possibly
	// mask user error if data is the complete wrong fit. instead we
	// can iterate over data and apply that to the container, and fail
	// early if there isn't a matching field.
	ref := reflect.ValueOf(container).Elem()
	typ := ref.Type()
	fields := ref.FieldByName("Fields")
	for k, v := range data {
		f := fields.FieldByName(k)
		val := reflect.ValueOf(v)
		if !f.IsValid() {
			errstr := fmt.Sprintf("cannot find field %s.%s", typ, k)
			panic(errstr)
		}
		if fkind, vkind := f.Kind(), val.Kind(); fkind != vkind {
			errstr := fmt.Sprintf("type error setting %s.%s: %s != %s", typ, k, fkind, vkind)
			panic(errstr)
		}
		f.Set(val)
	}
}

// Create ...
func (t *Table) Create(record interface{}) error {
	body, err := getJSONBody(record)
	if err != nil {
		return err
	}
	res, err := t.client.RequestWithBody("POST", t.makePath(""), Options{}, body)
	if err != nil {
		return err
	}
	return json.Unmarshal(res, record)
}

type deleteResponse struct {
	Deleted bool
	ID      string
}

func (t *Table) makePath(id string) string {
	n := url.PathEscape(t.name)
	if id == "" {
		return n
	}
	return path.Join(n, id)
}

// Delete ...
func (t *Table) Delete(record interface{}) error {
	id, err := getID(record)
	if err != nil {
		return err
	}
	res, err := t.client.Request("DELETE", t.makePath(id), Options{})
	if err != nil {
		return err
	}
	deleted := deleteResponse{}
	if err := json.Unmarshal(res, &deleted); err != nil {
		return err
	}
	if !deleted.Deleted {
		return fmt.Errorf("error: did not delete %s", res)
	}

	return nil
}

// List returns stuff
func (t *Table) List(listPtr interface{}, options *Options) error {
	if options == nil {
		options = &Options{}
	}

	oneRecord := reflect.TypeOf(listPtr).Elem().Elem()
	options.typ = oneRecord

	bytes, err := t.client.Request("GET", t.makePath(""), options)
	if err != nil {
		return err
	}

	responseType := reflect.StructOf([]reflect.StructField{
		{Name: "Records", Type: reflect.TypeOf(listPtr).Elem()},
		{Name: "Offset", Type: reflect.TypeOf("")},
	})

	container := reflect.New(responseType)
	err = json.Unmarshal(bytes, container.Interface())
	if err != nil {
		return err
	}

	recordList := container.Elem().FieldByName("Records")
	list := reflect.ValueOf(listPtr).Elem()
	for i := 0; i < recordList.Len(); i++ {
		entry := recordList.Index(i)
		list = reflect.Append(list, entry)
	}
	reflect.ValueOf(listPtr).Elem().Set(list)

	offset := container.Elem().FieldByName("Offset").String()
	if offset != "" {
		options.offset = offset
		return t.List(listPtr, options)
	}
	return nil
}

func getJSONBody(r interface{}) (io.Reader, error) {
	f, err := getFields(r)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	jsonstr := fmt.Sprintf(`{"fields": %s}`, b)
	body := strings.NewReader(jsonstr)
	return body, nil
}

func getFields(e interface{}) (interface{}, error) {
	fields := reflect.ValueOf(e).Elem().FieldByName("Fields")
	if !fields.IsValid() {
		return nil, errors.New("getFields: missing Fields")
	}
	if fields.Kind() != reflect.Struct {
		return nil, errors.New("getFields: Fields not a struct")
	}
	return fields.Interface(), nil
}

func getID(e interface{}) (string, error) {
	id := reflect.ValueOf(e).Elem().FieldByName("ID")
	if !id.IsValid() {
		return "", errors.New("getID: missing ID")
	}
	if id.Kind() != reflect.String {
		return "", errors.New("getID: ID not a string")
	}
	return id.String(), nil
}

func debugLog(s string, a ...interface{}) {
	fmt.Printf("DEBUG: "+s+"\n", a...)
}
