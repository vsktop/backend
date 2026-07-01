package account_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vsktop/backend/internal/domain/account"
)

type mockRepo struct {
	accounts map[string]*account.Account
}

func newMockRepo() *mockRepo {
	return &mockRepo{accounts: make(map[string]*account.Account)}
}

func (m *mockRepo) Create(_ context.Context, a *account.Account) error {
	if _, ok := m.accounts[a.AccountID]; ok {
		return account.ErrAlreadyExists
	}
	copy := *a
	m.accounts[a.AccountID] = &copy
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id string) (*account.Account, error) {
	a, ok := m.accounts[id]
	if !ok {
		return nil, account.ErrNotFound
	}
	copy := *a
	return &copy, nil
}

func (m *mockRepo) GetByIdentityKey(_ context.Context, key []byte) (*account.Account, error) {
	for _, a := range m.accounts {
		if string(a.IdentityKey) == string(key) {
			copy := *a
			return &copy, nil
		}
	}
	return nil, account.ErrNotFound
}

func (m *mockRepo) UpdateProfileBlob(_ context.Context, id string, blob []byte) error {
	a, ok := m.accounts[id]
	if !ok {
		return account.ErrNotFound
	}
	a.ProfileBlob = blob
	return nil
}

func (m *mockRepo) Suspend(_ context.Context, id string, reason string) error {
	a, ok := m.accounts[id]
	if !ok {
		return account.ErrNotFound
	}
	now := time.Now()
	a.SuspendedAt = &now
	a.SuspensionReason = &reason
	return nil
}

func (m *mockRepo) Unsuspend(_ context.Context, id string) error {
	a, ok := m.accounts[id]
	if !ok {
		return account.ErrNotFound
	}
	a.SuspendedAt = nil
	a.SuspensionReason = nil
	return nil
}

func (m *mockRepo) IsSuspended(_ context.Context, id string) (bool, error) {
	a, ok := m.accounts[id]
	if !ok {
		return false, account.ErrNotFound
	}
	return a.IsSuspended(), nil
}

func (m *mockRepo) Delete(_ context.Context, id string) error {
	if _, ok := m.accounts[id]; !ok {
		return account.ErrNotFound
	}
	delete(m.accounts, id)
	return nil
}

func (m *mockRepo) Exists(_ context.Context, id string) (bool, error) {
	_, ok := m.accounts[id]
	return ok, nil
}

func generateKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func buildValidRequest(t *testing.T, ikPub ed25519.PublicKey, ikPriv ed25519.PrivateKey) account.RegistrationRequest {
	t.Helper()

	devicePub, _ := generateKeypair(t)
	ts := time.Now().Unix()

	payload := make([]byte, ed25519.PublicKeySize+8)
	copy(payload, devicePub)
	binary.BigEndian.PutUint64(payload[ed25519.PublicKeySize:], uint64(ts))
	sig := ed25519.Sign(ikPriv, payload)

	challengeHash := make([]byte, sha256.Size)
	_, err := rand.Read(challengeHash)
	require.NoError(t, err)

	nonce := solvePoW(t, challengeHash, 1)

	return account.RegistrationRequest{
		IdentityKey:         ikPub,
		DevicePublicKey:     devicePub,
		DeviceCertSignature: sig,
		Timestamp:           ts,
		PowNonce:            nonce,
		PowHash:             challengeHash,
	}
}

func solvePoW(t *testing.T, challengeHash []byte, difficulty uint8) []byte {
	t.Helper()
	nonce := make([]byte, 8)
	for i := uint64(0); ; i++ {
		binary.BigEndian.PutUint64(nonce, i)
		input := append(nonce, challengeHash...)
		h := sha256.Sum256(input)
		if leadingZeroBits(h[:]) >= difficulty {
			result := make([]byte, 8)
			copy(result, nonce)
			return result
		}
	}
}

func leadingZeroBits(b []byte) uint8 {
	var count uint8
	for _, byt := range b {
		if byt == 0 {
			count += 8
			continue
		}
		for mask := byte(0x80); mask != 0; mask >>= 1 {
			if byt&mask != 0 {
				return count
			}
			count++
		}
		break
	}
	return count
}

func newService(t *testing.T) (*account.Service, *mockRepo) {
	t.Helper()
	repo := newMockRepo()
	svc := account.NewService(repo,
		account.WithPowDifficulty(1),
		account.WithMaxTimestampSkew(30*time.Second),
	)
	return svc, repo
}

func TestRegister_Success(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	a, err := svc.Register(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Len(t, a.AccountID, 64, "account_id must be 64 hex chars")
	assert.Equal(t, []byte(ikPub), a.IdentityKey)
	assert.True(t, a.IsActive())
	assert.False(t, a.IsSuspended())
}

func TestRegister_DuplicateIdentityKey(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)

	req := buildValidRequest(t, ikPub, ikPriv)
	_, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	req2 := buildValidRequest(t, ikPub, ikPriv)
	_, err = svc.Register(context.Background(), req2)

	assert.ErrorIs(t, err, account.ErrAlreadyExists)
}

func TestRegister_InvalidDeviceCertSignature(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	req.DeviceCertSignature[0] ^= 0xFF

	_, err := svc.Register(context.Background(), req)

	assert.Error(t, err)
	assert.NotErrorIs(t, err, account.ErrAlreadyExists)
}

func TestRegister_SignatureFromWrongKey(t *testing.T) {
	svc, _ := newService(t)
	ikPub, _ := generateKeypair(t)

	_, wrongPriv := generateKeypair(t)

	req := buildValidRequest(t, ikPub, wrongPriv)

	_, err := svc.Register(context.Background(), req)

	assert.Error(t, err)
}

func TestRegister_TimestampTooOld(t *testing.T) {
	repo := newMockRepo()

	now := time.Now()
	svc := account.NewService(repo,
		account.WithPowDifficulty(1),
		account.WithClock(func() time.Time { return now }),
		account.WithMaxTimestampSkew(30*time.Second),
	)

	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)
	req.Timestamp = now.Add(-60 * time.Second).Unix()

	payload := make([]byte, ed25519.PublicKeySize+8)
	copy(payload, req.DevicePublicKey)
	binary.BigEndian.PutUint64(payload[ed25519.PublicKeySize:], uint64(req.Timestamp))
	req.DeviceCertSignature = ed25519.Sign(ikPriv, payload)

	_, err := svc.Register(context.Background(), req)

	assert.Error(t, err)
}

func TestRegister_TimestampFromFuture(t *testing.T) {
	repo := newMockRepo()
	now := time.Now()
	svc := account.NewService(repo,
		account.WithPowDifficulty(1),
		account.WithClock(func() time.Time { return now }),
		account.WithMaxTimestampSkew(30*time.Second),
	)

	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)
	req.Timestamp = now.Add(60 * time.Second).Unix()

	payload := make([]byte, ed25519.PublicKeySize+8)
	copy(payload, req.DevicePublicKey)
	binary.BigEndian.PutUint64(payload[ed25519.PublicKeySize:], uint64(req.Timestamp))
	req.DeviceCertSignature = ed25519.Sign(ikPriv, payload)

	_, err := svc.Register(context.Background(), req)

	assert.Error(t, err)
}

func TestRegister_InvalidKeyLength(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	req.IdentityKey = req.IdentityKey[:16]

	_, err := svc.Register(context.Background(), req)

	assert.Error(t, err)
}

func TestRegister_PowMissing(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	req.PowNonce = nil

	_, err := svc.Register(context.Background(), req)

	assert.Error(t, err)
}

func TestGetByID_ActiveAccount(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	created, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	fetched, err := svc.GetByID(context.Background(), created.AccountID)

	require.NoError(t, err)
	assert.Equal(t, created.AccountID, fetched.AccountID)
}

func TestGetByID_NotFound(t *testing.T) {
	svc, _ := newService(t)

	_, err := svc.GetByID(context.Background(), "nonexistent000000000000000000000000000000000000000000000000000000")

	assert.ErrorIs(t, err, account.ErrNotFound)
}

func TestGetByID_SuspendedAccount(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	created, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	err = svc.Suspend(context.Background(), account.SuspendRequest{
		AccountID: created.AccountID,
		Reason:    "spam",
	})
	require.NoError(t, err)

	fetched, err := svc.GetByID(context.Background(), created.AccountID)

	assert.ErrorIs(t, err, account.ErrSuspended)
	assert.NotNil(t, fetched, "account should still be returned even when suspended")
}

func TestSuspend_InvalidReason(t *testing.T) {
	svc, _ := newService(t)

	err := svc.Suspend(context.Background(), account.SuspendRequest{
		AccountID: "someid",
		Reason:    "made_up_reason",
	})

	assert.Error(t, err)
}

func TestSuspend_UnknownAccount(t *testing.T) {
	svc, _ := newService(t)

	err := svc.Suspend(context.Background(), account.SuspendRequest{
		AccountID: "doesnotexist0000000000000000000000000000000000000000000000000000",
		Reason:    "spam",
	})

	assert.ErrorIs(t, err, account.ErrNotFound)
}

func TestUnsuspend_ReinstatesAccount(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	created, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	err = svc.Suspend(context.Background(), account.SuspendRequest{
		AccountID: created.AccountID,
		Reason:    "harassment",
	})
	require.NoError(t, err)

	err = svc.Unsuspend(context.Background(), created.AccountID)
	require.NoError(t, err)

	fetched, err := svc.GetByID(context.Background(), created.AccountID)
	require.NoError(t, err)
	assert.True(t, fetched.IsActive())
}

func TestUpdateProfile_Success(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	created, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	blob := []byte("encrypted-profile-data")
	err = svc.UpdateProfile(context.Background(), account.UpdateProfileRequest{
		AccountID:   created.AccountID,
		ProfileBlob: blob,
	})

	assert.NoError(t, err)
}

func TestUpdateProfile_BlobTooLarge(t *testing.T) {
	svc, _ := newService(t)

	bigBlob := make([]byte, 5000)

	err := svc.UpdateProfile(context.Background(), account.UpdateProfileRequest{
		AccountID:   "anyid",
		ProfileBlob: bigBlob,
	})

	assert.Error(t, err)
}

func TestDelete_AccountDisappears(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	created, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	err = svc.Delete(context.Background(), created.AccountID)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), created.AccountID)
	assert.ErrorIs(t, err, account.ErrNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	svc, _ := newService(t)

	err := svc.Delete(context.Background(), "nonexistent000000000000000000000000000000000000000000000000000000")

	assert.ErrorIs(t, err, account.ErrNotFound)
}

func TestAccountID_IsDeterministic(t *testing.T) {

	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)

	req1 := buildValidRequest(t, ikPub, ikPriv)
	a1, err := svc.Register(context.Background(), req1)
	require.NoError(t, err)

	hash := sha256.Sum256(ikPub)
	expected := make([]byte, sha256.Size)
	copy(expected, hash[:])

	assert.Len(t, a1.AccountID, 64)

	req2 := buildValidRequest(t, ikPub, ikPriv)
	_, err = svc.Register(context.Background(), req2)
	assert.ErrorIs(t, err, account.ErrAlreadyExists,
		"same identity key must produce same account_id and collide")
}

func TestIsSuspended_HotPath(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	created, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	suspended, err := svc.IsSuspended(context.Background(), created.AccountID)
	require.NoError(t, err)
	assert.False(t, suspended)

	err = svc.Suspend(context.Background(), account.SuspendRequest{
		AccountID: created.AccountID,
		Reason:    "spam",
	})
	require.NoError(t, err)

	suspended, err = svc.IsSuspended(context.Background(), created.AccountID)
	require.NoError(t, err)
	assert.True(t, suspended)
}

func TestExists(t *testing.T) {
	svc, _ := newService(t)
	ikPub, ikPriv := generateKeypair(t)
	req := buildValidRequest(t, ikPub, ikPriv)

	exists, err := svc.Exists(context.Background(), "doesnotexist00000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	assert.False(t, exists)

	created, err := svc.Register(context.Background(), req)
	require.NoError(t, err)

	exists, err = svc.Exists(context.Background(), created.AccountID)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestEmptyAccountID_Guards(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, "")
	assert.Error(t, err)

	err = svc.Suspend(ctx, account.SuspendRequest{AccountID: "", Reason: "spam"})
	assert.Error(t, err)

	err = svc.Unsuspend(ctx, "")
	assert.Error(t, err)

	err = svc.Delete(ctx, "")
	assert.Error(t, err)

	_, err = svc.Exists(ctx, "")
	assert.Error(t, err)

	_, err = svc.IsSuspended(ctx, "")
	assert.Error(t, err)

	err = svc.UpdateProfile(ctx, account.UpdateProfileRequest{AccountID: ""})
	assert.Error(t, err)
}

var _ account.Repository = (*mockRepo)(nil)
