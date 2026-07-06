package auth

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v3"

	"github.com/vsktop/backend/internal/domain/account"
	"github.com/vsktop/backend/internal/domain/device"
	"github.com/vsktop/backend/internal/domain/prekey"
)

type Handler struct {
	accounts *account.Service
	devices  *device.Service
	prekeys  *prekey.Service
}

func NewHandler(accounts *account.Service, devices *device.Service, prekeys *prekey.Service) *Handler {
	return &Handler{
		accounts: accounts,
		devices:  devices,
		prekeys:  prekeys,
	}
}

func (h *Handler) RegisterRoutes(r fiber.Router, mw ...fiber.Handler) {
	for _, m := range mw {
		r.Use(m)
	}
	r.Post("/accounts", h.Register)
	r.Get("/accounts/:account_id", h.GetAccount)
	r.Delete("/accounts/:account_id", h.DeleteAccount)

	r.Post("/accounts/:account_id/prekeys/signed", h.UploadSignedPreKey)
	r.Post("/accounts/:account_id/prekeys/one-time", h.UploadOneTimePreKeys)
	r.Get("/accounts/:account_id/devices/:device_id/prekeys", h.FetchPrekeyBundle)
}

type registerRequest struct {
	IdentityKey         string `json:"identity_key"`
	DevicePublicKey     string `json:"device_public_key"`
	DeviceCertSignature string `json:"device_cert_signature"`
	Timestamp           int64  `json:"timestamp"`
	PowNonce            string `json:"pow_nonce"`
	PowHash             string `json:"pow_hash"`
}

type registerResponse struct {
	AccountID string `json:"account_id"`
}

type accountResponse struct {
	AccountID   string `json:"account_id"`
	IdentityKey string `json:"identity_key"`
	ProfileBlob string `json:"profile_blob,omitempty"`
}

type uploadSPKRequest struct {
	DeviceID  string `json:"device_id"`
	KeyID     uint32 `json:"key_id"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type uploadOTKsRequest struct {
	DeviceID string `json:"device_id"`
	Keys     []struct {
		KeyID     uint32 `json:"key_id"`
		PublicKey string `json:"public_key"`
		Signature string `json:"signature"`
	} `json:"keys"`
}

type uploadOTKsResponse struct {
	Uploaded int `json:"uploaded"`
}

type prekeyBundleResponse struct {
	DeviceID    string `json:"device_id"`
	NeedsRefill bool   `json:"needs_refill"`

	SignedPreKey struct {
		KeyID     uint32 `json:"key_id"`
		PublicKey string `json:"public_key"`
		Signature string `json:"signature"`
	} `json:"signed_prekey"`

	OneTimePreKey *struct {
		KeyID     uint32 `json:"key_id"`
		PublicKey string `json:"public_key"`
		Signature string `json:"signature"`
	} `json:"one_time_prekey,omitempty"`
}

func (h *Handler) Register(c fiber.Ctx) error {
	var req registerRequest
	if err := c.Bind().Body(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	identityKey, err := decodeHex(req.IdentityKey, "identity_key")
	if err != nil {
		return err
	}
	devicePubKey, err := decodeHex(req.DevicePublicKey, "device_public_key")
	if err != nil {
		return err
	}
	certSig, err := decodeBase64(req.DeviceCertSignature, "device_cert_signature")
	if err != nil {
		return err
	}
	powNonce, err := decodeHex(req.PowNonce, "pow_nonce")
	if err != nil {
		return err
	}
	powHash, err := decodeHex(req.PowHash, "pow_hash")
	if err != nil {
		return err
	}

	a, err := h.accounts.Register(c.Context(), account.RegistrationRequest{
		IdentityKey:         identityKey,
		DevicePublicKey:     devicePubKey,
		DeviceCertSignature: certSig,
		Timestamp:           req.Timestamp,
		PowNonce:            powNonce,
		PowHash:             powHash,
	})
	if err != nil {
		if errors.Is(err, account.ErrAlreadyExists) {
			return fiber.NewError(fiber.StatusConflict, "account already exists")
		}
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(registerResponse{
		AccountID: a.AccountID,
	})
}

func (h *Handler) GetAccount(c fiber.Ctx) error {
	accountID := c.Params("account_id")
	if accountID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "account_id required")
	}

	a, err := h.accounts.GetByID(c.Context(), accountID)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "account not found")
		}
		if errors.Is(err, account.ErrSuspended) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error":      "account_suspended",
				"account_id": accountID,
			})
		}
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}

	resp := accountResponse{
		AccountID:   a.AccountID,
		IdentityKey: encodeHex(a.IdentityKey),
	}
	if len(a.ProfileBlob) > 0 {
		resp.ProfileBlob = encodeBase64(a.ProfileBlob)
	}

	return c.JSON(resp)
}

func (h *Handler) DeleteAccount(c fiber.Ctx) error {
	accountID := c.Params("account_id")
	if accountID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "account_id required")
	}

	if err := requireSelf(c, accountID); err != nil {
		return err
	}

	if err := h.accounts.Delete(c.Context(), accountID); err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "account not found")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) UploadSignedPreKey(c fiber.Ctx) error {
	accountID := c.Params("account_id")
	if err := requireSelf(c, accountID); err != nil {
		return err
	}

	var req uploadSPKRequest
	if err := c.Bind().Body(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.DeviceID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "device_id required")
	}

	pubKey, err := decodeHex(req.PublicKey, "public_key")
	if err != nil {
		return err
	}
	sig, err := decodeBase64(req.Signature, "signature")
	if err != nil {
		return err
	}

	deviceIK, err := h.deviceIdentityKey(c, req.DeviceID)
	if err != nil {
		return err
	}

	spk := &prekey.SignedPreKey{
		DeviceID:  req.DeviceID,
		KeyID:     req.KeyID,
		PublicKey: pubKey,
		Signature: sig,
	}

	if err := h.prekeys.UploadSignedPreKey(c.Context(), deviceIK, spk); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) UploadOneTimePreKeys(c fiber.Ctx) error {
	accountID := c.Params("account_id")
	if err := requireSelf(c, accountID); err != nil {
		return err
	}

	var req uploadOTKsRequest
	if err := c.Bind().Body(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if req.DeviceID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "device_id required")
	}
	if len(req.Keys) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "keys must not be empty")
	}

	deviceIK, err := h.deviceIdentityKey(c, req.DeviceID)
	if err != nil {
		return err
	}

	keys := make([]prekey.OneTimePreKey, len(req.Keys))
	for i, k := range req.Keys {
		pub, err := decodeHex(k.PublicKey, "public_key")
		if err != nil {
			return err
		}
		sig, err := decodeBase64(k.Signature, "signature")
		if err != nil {
			return err
		}
		keys[i] = prekey.OneTimePreKey{
			KeyID:     k.KeyID,
			PublicKey: pub,
			Signature: sig,
		}
	}

	if err := h.prekeys.UploadOneTimePreKeys(c.Context(), deviceIK, req.DeviceID, keys); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(uploadOTKsResponse{
		Uploaded: len(keys),
	})
}

func (h *Handler) FetchPrekeyBundle(c fiber.Ctx) error {
	deviceID := c.Params("device_id")
	if deviceID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "device_id required")
	}

	bundle, needsRefill, err := h.prekeys.FetchBundle(c.Context(), deviceID)
	if err != nil {
		if errors.Is(err, prekey.ErrNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "no prekeys available for device")
		}
		return fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}

	resp := prekeyBundleResponse{
		DeviceID:    bundle.DeviceID,
		NeedsRefill: needsRefill,
	}
	resp.SignedPreKey.KeyID = bundle.SPK.KeyID
	resp.SignedPreKey.PublicKey = encodeHex(bundle.SPK.PublicKey)
	resp.SignedPreKey.Signature = encodeBase64(bundle.SPK.Signature)

	if bundle.OTK != nil {
		resp.OneTimePreKey = &struct {
			KeyID     uint32 `json:"key_id"`
			PublicKey string `json:"public_key"`
			Signature string `json:"signature"`
		}{
			KeyID:     bundle.OTK.KeyID,
			PublicKey: encodeHex(bundle.OTK.PublicKey),
			Signature: encodeBase64(bundle.OTK.Signature),
		}
	}

	return c.JSON(resp)
}

func requireSelf(c fiber.Ctx, targetAccountID string) error {
	callerID, ok := c.Locals("account_id").(string)
	if !ok || callerID == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthenticated")
	}
	if callerID != targetAccountID {
		return fiber.NewError(fiber.StatusForbidden, "forbidden")
	}
	return nil
}

func (h *Handler) deviceIdentityKey(c fiber.Ctx, deviceID string) ([]byte, error) {
	d, err := h.devices.GetByID(c.Context(), deviceID)
	if err != nil {
		if errors.Is(err, device.ErrNotFound) {
			return nil, fiber.NewError(fiber.StatusNotFound, "device not found")
		}
		return nil, fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	if len(d.DeviceCert) < 32 {
		return nil, fiber.NewError(fiber.StatusInternalServerError, "internal error")
	}
	return d.DeviceCert[:32], nil
}

func decodeHex(s, field string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: invalid hex", field))
	}
	return b, nil
}

func decodeBase64(s, field string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("%s: invalid base64", field))
	}
	return b, nil
}

func encodeHex(b []byte) string {
	return hex.EncodeToString(b)
}

func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
