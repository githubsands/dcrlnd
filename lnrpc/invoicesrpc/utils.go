package invoicesrpc

import (
	"encoding/hex"
	"fmt"

	"github.com/decred/dcrd/chaincfg/v2"
	"github.com/decred/dcrlnd/channeldb"
	"github.com/decred/dcrlnd/lnrpc"
	"github.com/decred/dcrlnd/zpay32"
)

// CreateRPCInvoice creates an *lnrpc.Invoice from the *channeldb.Invoice.
func CreateRPCInvoice(invoice *channeldb.Invoice,
	activeNetParams *chaincfg.Params) (*lnrpc.Invoice, error) {

	paymentRequest := string(invoice.PaymentRequest)
	decoded, err := zpay32.Decode(paymentRequest, activeNetParams)
	if err != nil {
		return nil, fmt.Errorf("unable to decode payment request: %v",
			err)
	}

	var descHash []byte
	if decoded.DescriptionHash != nil {
		descHash = decoded.DescriptionHash[:]
	}

	fallbackAddr := ""
	if decoded.FallbackAddr != nil {
		fallbackAddr = decoded.FallbackAddr.String()
	}

	settleDate := int64(0)
	if !invoice.SettleDate.IsZero() {
		settleDate = invoice.SettleDate.Unix()
	}

	// Convert between the `lnrpc` and `routing` types.
	routeHints := CreateRPCRouteHints(decoded.RouteHints)

	preimage := invoice.Terms.PaymentPreimage
	atomsAmt := invoice.Terms.Value.ToAtoms()
	atomsAmtPaid := invoice.AmtPaid.ToAtoms()

	isSettled := invoice.Terms.State == channeldb.ContractSettled

	var state lnrpc.Invoice_InvoiceState
	switch invoice.Terms.State {
	case channeldb.ContractOpen:
		state = lnrpc.Invoice_OPEN
	case channeldb.ContractSettled:
		state = lnrpc.Invoice_SETTLED
	case channeldb.ContractCanceled:
		state = lnrpc.Invoice_CANCELED
	case channeldb.ContractAccepted:
		state = lnrpc.Invoice_ACCEPTED
	default:
		return nil, fmt.Errorf("unknown invoice state %v",
			invoice.Terms.State)
	}

	rpcHtlcs := make([]*lnrpc.InvoiceHTLC, 0, len(invoice.Htlcs))
	for key, htlc := range invoice.Htlcs {
		var state lnrpc.InvoiceHTLCState
		switch htlc.State {
		case channeldb.HtlcStateAccepted:
			state = lnrpc.InvoiceHTLCState_ACCEPTED
		case channeldb.HtlcStateSettled:
			state = lnrpc.InvoiceHTLCState_SETTLED
		case channeldb.HtlcStateCanceled:
			state = lnrpc.InvoiceHTLCState_CANCELED
		default:
			return nil, fmt.Errorf("unknown state %v", htlc.State)
		}

		rpcHtlc := lnrpc.InvoiceHTLC{
			ChanId:       key.ChanID.ToUint64(),
			HtlcIndex:    key.HtlcID,
			AcceptHeight: int32(htlc.AcceptHeight),
			AcceptTime:   htlc.AcceptTime.Unix(),
			ExpiryHeight: int32(htlc.Expiry),
			AmtMAtoms:    uint64(htlc.Amt),
			State:        state,
		}

		// Only report resolved times if htlc is resolved.
		if htlc.State != channeldb.HtlcStateAccepted {
			rpcHtlc.ResolveTime = htlc.ResolveTime.Unix()
		}

		rpcHtlcs = append(rpcHtlcs, &rpcHtlc)
	}

	rpcInvoice := &lnrpc.Invoice{
		Memo:            string(invoice.Memo[:]),
		Receipt:         invoice.Receipt[:],
		RHash:           decoded.PaymentHash[:],
		Value:           int64(atomsAmt),
		CreationDate:    invoice.CreationDate.Unix(),
		SettleDate:      settleDate,
		Settled:         isSettled,
		PaymentRequest:  paymentRequest,
		DescriptionHash: descHash,
		Expiry:          int64(invoice.Expiry.Seconds()),
		CltvExpiry:      uint64(invoice.FinalCltvDelta),
		FallbackAddr:    fallbackAddr,
		RouteHints:      routeHints,
		AddIndex:        invoice.AddIndex,
		Private:         len(routeHints) > 0,
		SettleIndex:     invoice.SettleIndex,
		AmtPaidAtoms:    int64(atomsAmtPaid),
		AmtPaidMAtoms:   int64(invoice.AmtPaid),
		AmtPaid:         int64(invoice.AmtPaid),
		State:           state,
		Htlcs:           rpcHtlcs,
	}

	if preimage != channeldb.UnknownPreimage {
		rpcInvoice.RPreimage = preimage[:]
	}

	return rpcInvoice, nil
}

// CreateRPCRouteHints takes in the decoded form of an invoice's route hints
// and converts them into the lnrpc type.
func CreateRPCRouteHints(routeHints [][]zpay32.HopHint) []*lnrpc.RouteHint {
	var res []*lnrpc.RouteHint

	for _, route := range routeHints {
		hopHints := make([]*lnrpc.HopHint, 0, len(route))
		for _, hop := range route {
			pubKey := hex.EncodeToString(
				hop.NodeID.SerializeCompressed(),
			)

			hint := &lnrpc.HopHint{
				NodeId:                    pubKey,
				ChanId:                    hop.ChannelID,
				FeeBaseMAtoms:             hop.FeeBaseMAtoms,
				FeeProportionalMillionths: hop.FeeProportionalMillionths,
				CltvExpiryDelta:           uint32(hop.CLTVExpiryDelta),
			}

			hopHints = append(hopHints, hint)
		}

		routeHint := &lnrpc.RouteHint{HopHints: hopHints}
		res = append(res, routeHint)
	}

	return res
}
