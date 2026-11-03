package actionprotocol

import "encoding/json"

type ClaimRequest struct {
	ClaimNonce        string   `json:"claim_nonce"`
	EligibleTargetIDs []string `json:"eligible_target_ids"`
}

type ClaimResponse struct {
	ClaimNonce string `json:"claim_nonce"`
	Claim      Claim  `json:"claim"`
}

type CheckRequest struct {
	Request          RequestReceipt `json:"request"`
	ExecutionLeaseID string         `json:"execution_lease_id"`
	Checkpoint       string         `json:"checkpoint"`
	CheckNonce       string         `json:"check_nonce"`
}

func DecodeClaimRequest(raw []byte) (ClaimRequest, error) {
	var request ClaimRequest
	if _, err := fields(raw, "claim_nonce", "eligible_target_ids"); err != nil || json.Unmarshal(raw, &request) != nil || !nonce(request.ClaimNonce) ||
		len(request.EligibleTargetIDs) < 1 || len(request.EligibleTargetIDs) > 32 {
		return ClaimRequest{}, ErrInvalid
	}
	previous := ""
	for _, target := range request.EligibleTargetIDs {
		if !identifier(target, "hlt_") || target <= previous {
			return ClaimRequest{}, ErrInvalid
		}
		previous = target
	}
	return request, nil
}

func DecodeClaimResponse(raw []byte) (ClaimResponse, error) {
	var response ClaimResponse
	object, err := fields(raw, "claim_nonce", "claim")
	if err != nil || json.Unmarshal(raw, &response) != nil || !nonce(response.ClaimNonce) {
		return ClaimResponse{}, ErrInvalid
	}
	response.Claim, err = DecodeClaim(object["claim"])
	if err != nil {
		return ClaimResponse{}, ErrInvalid
	}
	return response, nil
}

func DecodeCheckRequest(raw []byte) (CheckRequest, error) {
	var request CheckRequest
	object, err := fields(raw, "request", "execution_lease_id", "checkpoint", "check_nonce")
	if err != nil || json.Unmarshal(raw, &request) != nil || !identifier(request.ExecutionLeaseID, "hle_") ||
		(request.Checkpoint != "1" && request.Checkpoint != "2") || !nonce(request.CheckNonce) {
		return CheckRequest{}, ErrInvalid
	}
	request.Request, err = DecodeRequest(object["request"])
	if err != nil {
		return CheckRequest{}, ErrInvalid
	}
	return request, nil
}
