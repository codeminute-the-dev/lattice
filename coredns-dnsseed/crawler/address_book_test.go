package crawler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddressBook_AddAndCount(t *testing.T) {
	ab := NewAddressBook()
	assert.Equal(t, 0, ab.Count())

	ab.Add("127.0.0.1:44108")
	assert.Equal(t, 1, ab.Count())

	ab.Add("127.0.0.2:44108")
	assert.Equal(t, 2, ab.Count())

	// Duplicate should overwrite, not increase count.
	ab.Add("127.0.0.1:44108")
	assert.Equal(t, 2, ab.Count())
}

func TestAddressBook_Remove(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")
	require.Equal(t, 1, ab.Count())

	ab.Remove("127.0.0.1:44108")
	assert.Equal(t, 0, ab.Count())

	// Removing a non-existent key is a no-op.
	ab.Remove("10.0.0.1:44108")
	assert.Equal(t, 0, ab.Count())
}

func TestAddressBook_IsKnown(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")

	assert.True(t, ab.IsKnown("127.0.0.1:44108"))
	assert.False(t, ab.IsKnown("10.0.0.1:44108"))
}

func TestAddressBook_Blacklist(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")
	require.Equal(t, 1, ab.Count())
	require.False(t, ab.IsBlacklisted("127.0.0.1:44108"))

	ab.Blacklist("127.0.0.1:44108")
	assert.Equal(t, 0, ab.Count())
	assert.True(t, ab.IsBlacklisted("127.0.0.1:44108"))
	assert.True(t, ab.IsKnown("127.0.0.1:44108"))
}

func TestAddressBook_BlacklistUnknownPeer(t *testing.T) {
	ab := NewAddressBook()
	ab.Blacklist("10.0.0.1:44108")

	assert.True(t, ab.IsBlacklisted("10.0.0.1:44108"))
	assert.True(t, ab.IsKnown("10.0.0.1:44108"))
	assert.Equal(t, 0, ab.Count())
}

func TestAddressBook_Redeem(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")
	ab.Blacklist("127.0.0.1:44108")
	require.True(t, ab.IsBlacklisted("127.0.0.1:44108"))

	ab.Redeem("127.0.0.1:44108")
	assert.False(t, ab.IsBlacklisted("127.0.0.1:44108"))
	assert.Equal(t, 1, ab.Count())
}

func TestAddressBook_DropFromBlacklist(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")
	ab.Blacklist("127.0.0.1:44108")

	ab.DropFromBlacklist("127.0.0.1:44108")
	assert.False(t, ab.IsBlacklisted("127.0.0.1:44108"))
	assert.False(t, ab.IsKnown("127.0.0.1:44108"))
}

func TestAddressBook_Touch(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")

	ab.mu.RLock()
	before := ab.peers["127.0.0.1:44108"].lastUpdate
	ab.mu.RUnlock()

	ab.Touch("127.0.0.1:44108")

	ab.mu.RLock()
	after := ab.peers["127.0.0.1:44108"].lastUpdate
	ab.mu.RUnlock()

	assert.False(t, after.Before(before))
}

func TestAddressBook_ShuffleAddressList(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")
	ab.Add("127.0.0.2:44108")
	ab.Add("127.0.0.3:44108")
	// Non-standard port; should be excluded.
	ab.Add("127.0.0.4:9999")

	ips := ab.ShuffleAddressList(10, false, "44108")
	assert.Len(t, ips, 3)

	// Max cap works.
	ips = ab.ShuffleAddressList(2, false, "44108")
	assert.Len(t, ips, 2)
}

func TestAddressBook_ShuffleAddressListV6(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")
	ab.Add("[::1]:44108")

	v4 := ab.ShuffleAddressList(10, false, "44108")
	v6 := ab.ShuffleAddressList(10, true, "44108")

	assert.Len(t, v4, 1)
	assert.Len(t, v6, 1)
}

func TestAddressBook_ShuffleExcludesBlacklisted(t *testing.T) {
	ab := NewAddressBook()
	ab.Add("127.0.0.1:44108")
	ab.Add("127.0.0.2:44108")
	ab.Blacklist("127.0.0.1:44108")

	ips := ab.ShuffleAddressList(10, false, "44108")
	assert.Len(t, ips, 1)
}

func TestAddressBook_WaitForAddresses(t *testing.T) {
	ab := NewAddressBook()
	ctx := context.Background()
	result := make(chan bool, 1)
	go func() {
		result <- ab.WaitForAddresses(ctx, 2)
	}()

	// Give the goroutine time to acquire the lock and enter Wait().
	time.Sleep(50 * time.Millisecond)

	ab.Add("127.0.0.1:44108")
	ab.Add("127.0.0.2:44108")

	select {
	case ok := <-result:
		assert.True(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForAddresses should have unblocked")
	}
}

func TestAddressBook_WaitForAddresses_Cancelled(t *testing.T) {
	ab := NewAddressBook()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan bool, 1)
	go func() {
		result <- ab.WaitForAddresses(ctx, 100)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case ok := <-result:
		assert.False(t, ok)
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForAddresses should have been cancelled")
	}
}
