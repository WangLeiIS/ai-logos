package book

import (
	"fmt"
	"io"
	"os"

	"github.com/parquet-go/parquet-go"
)

const (
	chunksFile = "chunks.parquet"
	indexFile  = "inverted_index.parquet"
	idfFile    = "idf_stats.parquet"
)

func ReadChunks(bundleDir string) ([]ChunkRow, error) {
	return readParquet[ChunkRow](bundleDir, chunksFile)
}

func ReadIndex(bundleDir string) ([]IndexRow, error) {
	return readParquet[IndexRow](bundleDir, indexFile)
}

func ReadIDF(bundleDir string) ([]IDFRow, error) {
	return readParquet[IDFRow](bundleDir, idfFile)
}

func FastValidate(bundleDir string) (*Manifest, error) {
	manifest, err := LoadManifest(bundleDir)
	if err != nil {
		return nil, err
	}
	if err := validateBundleParquetSchema[ChunkRow](bundleDir, chunksFile); err != nil {
		return nil, err
	}
	if err := validateBundleParquetSchema[IndexRow](bundleDir, indexFile); err != nil {
		return nil, err
	}
	if err := validateBundleParquetSchema[IDFRow](bundleDir, idfFile); err != nil {
		return nil, err
	}
	return manifest, nil
}

func readParquet[T any](bundleDir, name string) ([]T, error) {
	rows := make([]T, 0)
	if err := scanParquetRows[T](bundleDir, name, func(row T) error {
		rows = append(rows, row)
		return nil
	}); err != nil {
		return nil, err
	}
	return rows, nil
}

func scanParquetRows[T any](bundleDir, name string, visit func(T) error) error {
	file, err := openValidatedFile(bundleDir, name)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer file.Close()
	if err := validateParquetSchema[T](file); err != nil {
		return fmt.Errorf("validate %s schema: %w", name, err)
	}
	reader := parquet.NewGenericReader[T](file)
	defer reader.Close()
	buffer := make([]T, 64)
	for {
		n, err := reader.Read(buffer)
		for i := 0; i < n; i++ {
			if visitErr := visit(buffer[i]); visitErr != nil {
				return visitErr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
	}
}

func validateBundleParquetSchema[T any](bundleDir, name string) error {
	file, err := openValidatedFile(bundleDir, name)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer file.Close()
	if err := validateParquetSchema[T](file); err != nil {
		return fmt.Errorf("validate %s schema: %w", name, err)
	}
	return nil
}

func validateParquetSchema[T any](file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	actual, err := parquet.OpenFile(file, info.Size())
	if err != nil {
		return err
	}
	expected := parquet.SchemaOf(new(T))
	if err := requireFields(expected.Fields(), actual.Schema().Fields(), ""); err != nil {
		return fmt.Errorf("incompatible schema: %w", err)
	}
	return nil
}

// requireFields validates the fields required by the reader while allowing a
// Parquet file to contain additional metadata columns or use a different order.
func requireFields(required, actual []parquet.Field, prefix string) error {
	actualByName := make(map[string]parquet.Field, len(actual))
	for _, field := range actual {
		actualByName[field.Name()] = field
	}

	for _, want := range required {
		path := want.Name()
		if prefix != "" {
			path = prefix + "." + path
		}
		got, exists := actualByName[want.Name()]
		if !exists {
			return fmt.Errorf("missing required field %q", path)
		}
		if want.Leaf() != got.Leaf() || want.Repeated() != got.Repeated() {
			return fmt.Errorf("field %q has incompatible structure", path)
		}
		if want.Leaf() {
			if want.Type().Kind() != got.Type().Kind() {
				return fmt.Errorf("field %q has type %s, want %s", path, got.Type(), want.Type())
			}
		} else if err := requireFields(want.Fields(), got.Fields(), path); err != nil {
			return err
		}
	}
	return nil
}
