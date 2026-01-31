package jsonutil_test

import (
	"fmt"
	"os"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/mallardduck/dirio/internal/jsonutil"
)

// ExampleMarshal demonstrates automatic formatting based on environment
func ExampleMarshal() {
	type Config struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	config := Config{
		Name:    "dirio",
		Version: "1.0.0",
	}

	// Automatically formats based on DIRIO_DEBUG environment variable
	data, err := jsonutil.Marshal(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Output: %s\n", string(data))
	// Output in production (DIRIO_DEBUG unset): {"name":"dirio","version":"1.0.0"}
	// Output in debug (DIRIO_DEBUG=1):
	// {
	//   "name": "dirio",
	//   "version": "1.0.0"
	// }
}

// ExampleMarshalToFile demonstrates writing JSON to a file
func ExampleMarshalToFile() {
	type UserMetadata struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}

	fs := memfs.New()
	user := UserMetadata{
		Username: "john",
		Email:    "john@example.com",
	}

	// Write to file with automatic formatting
	err := jsonutil.MarshalToFile(fs, "user.json", user)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("User metadata saved successfully")
	// Output: User metadata saved successfully
}

// Example_migration shows how to migrate from old code to new jsonutil package
func Example_migration() {
	// OLD CODE (inconsistent):
	// import "encoding/json"
	//
	// // Some files use MarshalIndent
	// userData, err := json.MarshalIndent(user, "", "  ")
	// if err != nil {
	//     return err
	// }
	// util.WriteFile(fs, "user.json", userData, 0644)
	//
	// // Other files use Marshal
	// bucketData, err := json.Marshal(bucket)
	// if err != nil {
	//     return err
	// }
	// util.WriteFile(fs, "bucket.json", bucketData, 0644)

	// NEW CODE (consistent, automatic):
	fs := memfs.New()

	type User struct {
		Name string `json:"name"`
	}
	type Bucket struct {
		Name string `json:"name"`
	}

	user := User{Name: "john"}
	bucket := Bucket{Name: "my-bucket"}

	// Both use the same function, formatting is automatic
	jsonutil.MarshalToFile(fs, "user.json", user)
	jsonutil.MarshalToFile(fs, "bucket.json", bucket)

	fmt.Println("Migration complete")
	// Output: Migration complete
}

// Example_environmentControl demonstrates how to control formatting
func Example_environmentControl() {
	type Config struct {
		Debug bool `json:"debug"`
	}

	config := Config{Debug: true}

	// Save original env
	original := os.Getenv("DIRIO_DEBUG")
	defer func() {
		if original == "" {
			os.Unsetenv("DIRIO_DEBUG")
		} else {
			os.Setenv("DIRIO_DEBUG", original)
		}
	}()

	// Production mode (compact)
	os.Unsetenv("DIRIO_DEBUG")
	prodData, _ := jsonutil.Marshal(config)
	fmt.Printf("Production: %s\n", string(prodData))

	// Debug mode (pretty)
	os.Setenv("DIRIO_DEBUG", "1")
	debugData, _ := jsonutil.Marshal(config)
	fmt.Printf("Debug: %s\n", string(debugData))

	// Output:
	// Production: {"debug":true}
	// Debug: {
	//   "debug": true
	// }
}
