package main

import (
	"testing"
)

func TestNewOrderedMap(t *testing.T) {
	om := NewOrderedMap()
	if om == nil {
		t.Fatal("Expected OrderedMap to be initialized")
	}
	if len(om.keys) != 0 {
		t.Fatal("Expected keys to be empty")
	}
	if len(om.data) != 0 {
		t.Fatal("Expected data to be empty")
	}
}

func TestSet(t *testing.T) {
	om := NewOrderedMap()
	om.Set("key1", "value1")
	if len(om.keys) != 1 {
		t.Fatal("Expected keys length to be 1")
	}
	if om.data["key1"] != "value1" {
		t.Fatal("Expected value for 'key1' to be 'value1'")
	}
}

func TestGet(t *testing.T) {
	om := NewOrderedMap()
	om.Set("key1", "value1")
	value, exists := om.Get("key1")
	if !exists {
		t.Fatal("Expected key1 to exist")
	}
	if value != "value1" {
		t.Fatal("Expected value for 'key1' to be 'value1'")
	}
}

func TestKeys(t *testing.T) {
	om := NewOrderedMap()
	om.Set("key1", "value1")
	om.Set("key2", "value2")
	keys := om.Keys()
	if len(keys) != 2 {
		t.Fatal("Expected keys length to be 2")
	}
	if keys[0] != "key1" || keys[1] != "key2" {
		t.Fatal("Expected keys to be ['key1', 'key2']")
	}
}

func TestData(t *testing.T) {
	om := NewOrderedMap()
	om.Set("key1", "value1")
	data := om.Data()
	if len(data) != 1 {
		t.Fatal("Expected data length to be 1")
	}
	if data["key1"] != "value1" {
		t.Fatal("Expected value for 'key1' to be 'value1'")
	}
}

func TestRemove(t *testing.T) {
	om := NewOrderedMap()
	om.Set("key1", "value1")
	om.Remove("key1")
	if len(om.keys) != 0 {
		t.Fatal("Expected keys length to be 0")
	}
	if _, exists := om.data["key1"]; exists {
		t.Fatal("Expected 'key1' to be removed")
	}
}

func TestClear(t *testing.T) {
	om := NewOrderedMap()
	om.Set("key1", "value1")
	om.Clear()
	if len(om.keys) != 0 {
		t.Fatal("Expected keys length to be 0 after clear")
	}
	if len(om.data) != 0 {
		t.Fatal("Expected data length to be 0 after clear")
	}
}

func TestIsEmpty(t *testing.T) {
	om := NewOrderedMap()
	if !om.IsEmpty() {
		t.Fatal("Expected OrderedMap to be empty")
	}
	om.Set("key1", "value1")
	if om.IsEmpty() {
		t.Fatal("Expected OrderedMap to not be empty")
	}
}
