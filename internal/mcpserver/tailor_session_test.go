package mcpserver

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/thedandano/go-apply/internal/model"
)

func TestTailorSessionStore_HappyPath(t *testing.T) {
	store := NewTailorSessionStore()
	input := &model.TailorInput{ResumeText: "test resume"}

	id, err := store.Open("bundle", input, 5*time.Second)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}

	want := model.TailorResult{TailoredText: "tailored resume text"}

	done := make(chan struct{})
	var waitResult model.TailorResult
	var waitErr error
	go func() {
		defer close(done)
		waitResult, waitErr = store.Wait(context.Background(), id)
	}()

	if err := store.Submit(id, &want); err != nil {
		t.Fatalf("Submit: unexpected error: %v", err)
	}

	<-done

	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if waitResult.TailoredText != want.TailoredText {
		t.Errorf("Wait result TailoredText = %q, want %q", waitResult.TailoredText, want.TailoredText)
	}
}

func TestTailorSessionStore_Timeout(t *testing.T) {
	store := NewTailorSessionStore()
	input := &model.TailorInput{}

	id, err := store.Open("bundle", input, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}

	_, err = store.Wait(context.Background(), id)
	if !errors.Is(err, ErrTailorSessionExpired) {
		t.Errorf("Wait error = %v, want ErrTailorSessionExpired", err)
	}
}

func TestTailorSessionStore_SubmitUnknown(t *testing.T) {
	store := NewTailorSessionStore()

	r := model.TailorResult{}
	err := store.Submit("nonexistent-id", &r)
	if !errors.Is(err, ErrTailorSessionUnknown) {
		t.Errorf("Submit error = %v, want ErrTailorSessionUnknown", err)
	}
}

func TestTailorSessionStore_SubmitTwice(t *testing.T) {
	store := NewTailorSessionStore()
	input := &model.TailorInput{}

	id, err := store.Open("bundle", input, 5*time.Second)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}

	waitDone := make(chan struct{})
	go func() {
		defer close(waitDone)
		_, _ = store.Wait(context.Background(), id)
	}()

	first := model.TailorResult{TailoredText: "first"}
	if err := store.Submit(id, &first); err != nil {
		t.Fatalf("first Submit: unexpected error: %v", err)
	}

	<-waitDone

	second := model.TailorResult{TailoredText: "second"}
	err = store.Submit(id, &second)
	if !errors.Is(err, ErrTailorSessionAlreadyConsumed) {
		t.Errorf("second Submit error = %v, want ErrTailorSessionAlreadyConsumed", err)
	}
}

func TestTailorSessionStore_SubmitAfterExpiry(t *testing.T) {
	store := NewTailorSessionStore()
	input := &model.TailorInput{}

	id, err := store.Open("bundle", input, 1*time.Millisecond)
	if err != nil {
		t.Fatalf("Open: unexpected error: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	expired := model.TailorResult{}
	err = store.Submit(id, &expired)
	if !errors.Is(err, ErrTailorSessionExpired) {
		t.Errorf("Submit after expiry error = %v, want ErrTailorSessionExpired", err)
	}
}

func TestTailorSessionStore_Concurrent(t *testing.T) {
	const n = 20
	store := NewTailorSessionStore()
	input := &model.TailorInput{}

	ids := make([]string, n)
	for i := range n {
		id, err := store.Open("bundle", input, 5*time.Second)
		if err != nil {
			t.Fatalf("Open[%d]: unexpected error: %v", i, err)
		}
		ids[i] = id
	}

	var wg sync.WaitGroup
	wg.Add(2 * n)

	submitErrs := make([]error, n)
	waitErrs := make([]error, n)

	for i := range n {
		i := i
		go func() {
			defer wg.Done()
			r := model.TailorResult{TailoredText: "result"}
			submitErrs[i] = store.Submit(ids[i], &r)
		}()
		go func() {
			defer wg.Done()
			_, waitErrs[i] = store.Wait(context.Background(), ids[i])
		}()
	}

	wg.Wait()

	for i := range n {
		if submitErrs[i] != nil {
			t.Errorf("Submit[%d] error = %v", i, submitErrs[i])
		}
		if waitErrs[i] != nil {
			t.Errorf("Wait[%d] error = %v", i, waitErrs[i])
		}
	}
}
