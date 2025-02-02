package channeldb

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/dcrec/secp256k1/v2"
	"github.com/decred/dcrlnd/lntypes"
	"github.com/decred/dcrlnd/lnwire"
	"github.com/decred/dcrlnd/routing/route"
	"github.com/decred/dcrlnd/tlv"
)

var (
	priv, _ = secp256k1.GeneratePrivateKey()
	pub     = priv.PubKey()

	tlvBytes   = []byte{1, 2, 3}
	tlvEncoder = tlv.StubEncoder(tlvBytes)
	testHop1   = &route.Hop{
		PubKeyBytes:      route.NewVertex(pub),
		ChannelID:        12345,
		OutgoingTimeLock: 111,
		AmtToForward:     555,
		TLVRecords: []tlv.Record{
			tlv.MakeStaticRecord(1, nil, 3, tlvEncoder, nil),
			tlv.MakeStaticRecord(2, nil, 3, tlvEncoder, nil),
		},
	}

	testHop2 = &route.Hop{
		PubKeyBytes:      route.NewVertex(pub),
		ChannelID:        12345,
		OutgoingTimeLock: 111,
		AmtToForward:     555,
		LegacyPayload:    true,
	}

	testRoute = route.Route{
		TotalTimeLock: 123,
		TotalAmount:   1234567,
		SourcePubKey:  route.NewVertex(pub),
		Hops: []*route.Hop{
			testHop1,
			testHop2,
		},
	}
)

func makeFakePayment() *outgoingPayment {
	fakeInvoice := &Invoice{
		// Use single second precision to avoid false positive test
		// failures due to the monotonic time component.
		CreationDate:   time.Unix(time.Now().Unix(), 0),
		Memo:           []byte("fake memo"),
		Receipt:        []byte("fake receipt"),
		PaymentRequest: []byte(""),
	}

	copy(fakeInvoice.Terms.PaymentPreimage[:], rev[:])
	fakeInvoice.Terms.Value = lnwire.NewMAtomsFromAtoms(10000)

	fakePath := make([][33]byte, 3)
	for i := 0; i < 3; i++ {
		copy(fakePath[i][:], bytes.Repeat([]byte{byte(i)}, 33))
	}

	fakePayment := &outgoingPayment{
		Invoice:        *fakeInvoice,
		Fee:            101,
		Path:           fakePath,
		TimeLockLength: 1000,
	}
	copy(fakePayment.PaymentPreimage[:], rev[:])
	return fakePayment
}

func makeFakeInfo() (*PaymentCreationInfo, *PaymentAttemptInfo) {
	var preimg lntypes.Preimage
	copy(preimg[:], rev[:])

	c := &PaymentCreationInfo{
		PaymentHash: preimg.Hash(),
		Value:       1000,
		// Use single second precision to avoid false positive test
		// failures due to the monotonic time component.
		CreationDate:   time.Unix(time.Now().Unix(), 0),
		PaymentRequest: []byte(""),
	}

	a := &PaymentAttemptInfo{
		PaymentID:  44,
		SessionKey: priv,
		Route:      testRoute,
	}
	return c, a
}

// randomBytes creates random []byte with length in range [minLen, maxLen)
func randomBytes(minLen, maxLen int) ([]byte, error) {
	randBuf := make([]byte, minLen+rand.Intn(maxLen-minLen))

	if _, err := rand.Read(randBuf); err != nil {
		return nil, fmt.Errorf("Internal error. "+
			"Cannot generate random string: %v", err)
	}

	return randBuf, nil
}

func makeRandomFakePayment() (*outgoingPayment, error) {
	var err error
	fakeInvoice := &Invoice{
		// Use single second precision to avoid false positive test
		// failures due to the monotonic time component.
		CreationDate: time.Unix(time.Now().Unix(), 0),
	}

	fakeInvoice.Memo, err = randomBytes(1, 50)
	if err != nil {
		return nil, err
	}

	fakeInvoice.Receipt, err = randomBytes(1, 50)
	if err != nil {
		return nil, err
	}

	fakeInvoice.PaymentRequest, err = randomBytes(1, 50)
	if err != nil {
		return nil, err
	}

	preImg, err := randomBytes(32, 33)
	if err != nil {
		return nil, err
	}
	copy(fakeInvoice.Terms.PaymentPreimage[:], preImg)

	fakeInvoice.Terms.Value = lnwire.MilliAtom(rand.Intn(10000))

	fakePathLen := 1 + rand.Intn(5)
	fakePath := make([][33]byte, fakePathLen)
	for i := 0; i < fakePathLen; i++ {
		b, err := randomBytes(33, 34)
		if err != nil {
			return nil, err
		}
		copy(fakePath[i][:], b)
	}

	fakePayment := &outgoingPayment{
		Invoice:        *fakeInvoice,
		Fee:            lnwire.MilliAtom(rand.Intn(1001)),
		Path:           fakePath,
		TimeLockLength: uint32(rand.Intn(10000)),
	}
	copy(fakePayment.PaymentPreimage[:], fakeInvoice.Terms.PaymentPreimage[:])

	return fakePayment, nil
}

func TestSentPaymentSerialization(t *testing.T) {
	t.Parallel()

	c, s := makeFakeInfo()

	var b bytes.Buffer
	if err := serializePaymentCreationInfo(&b, c); err != nil {
		t.Fatalf("unable to serialize creation info: %v", err)
	}

	newCreationInfo, err := deserializePaymentCreationInfo(&b)
	if err != nil {
		t.Fatalf("unable to deserialize creation info: %v", err)
	}

	if !reflect.DeepEqual(c, newCreationInfo) {
		t.Fatalf("Payments do not match after "+
			"serialization/deserialization %v vs %v",
			spew.Sdump(c), spew.Sdump(newCreationInfo),
		)
	}

	b.Reset()
	if err := serializePaymentAttemptInfo(&b, s); err != nil {
		t.Fatalf("unable to serialize info: %v", err)
	}

	newAttemptInfo, err := deserializePaymentAttemptInfo(&b)
	if err != nil {
		t.Fatalf("unable to deserialize info: %v", err)
	}

	// First we verify all the records match up porperly, as they aren't
	// able to be properly compared using reflect.DeepEqual.
	err = assertRouteEqual(&s.Route, &newAttemptInfo.Route)
	if err != nil {
		t.Fatalf("Routes do not match after "+
			"serialization/deserialization: %v", err)
	}

	// Clear routes to allow DeepEqual to compare the remaining fields.
	newAttemptInfo.Route = route.Route{}
	s.Route = route.Route{}

	if !reflect.DeepEqual(s, newAttemptInfo) {
		s.SessionKey.Curve = nil
		newAttemptInfo.SessionKey.Curve = nil
		t.Fatalf("Payments do not match after "+
			"serialization/deserialization %v vs %v",
			spew.Sdump(s), spew.Sdump(newAttemptInfo),
		)
	}
}

// assertRouteEquals compares to routes for equality and returns an error if
// they are not equal.
func assertRouteEqual(a, b *route.Route) error {
	err := assertRouteHopRecordsEqual(a, b)
	if err != nil {
		return err
	}

	// TLV records have already been compared and need to be cleared to
	// properly compare the remaining fields using DeepEqual.
	copyRouteNoHops := func(r *route.Route) *route.Route {
		copy := *r
		copy.Hops = make([]*route.Hop, len(r.Hops))
		for i, hop := range r.Hops {
			hopCopy := *hop
			hopCopy.TLVRecords = nil
			copy.Hops[i] = &hopCopy
		}
		return &copy
	}

	if !reflect.DeepEqual(copyRouteNoHops(a), copyRouteNoHops(b)) {
		return fmt.Errorf("PaymentAttemptInfos don't match: %v vs %v",
			spew.Sdump(a), spew.Sdump(b))
	}

	return nil
}

func assertRouteHopRecordsEqual(r1, r2 *route.Route) error {
	if len(r1.Hops) != len(r2.Hops) {
		return errors.New("route hop count mismatch")
	}

	for i := 0; i < len(r1.Hops); i++ {
		records1 := r1.Hops[i].TLVRecords
		records2 := r2.Hops[i].TLVRecords
		if len(records1) != len(records2) {
			return fmt.Errorf("route record count for hop %v "+
				"mismatch", i)
		}

		for j := 0; j < len(records1); j++ {
			expectedRecord := records1[j]
			newRecord := records2[j]

			err := assertHopRecordsEqual(expectedRecord, newRecord)
			if err != nil {
				return fmt.Errorf("route record mismatch: %v", err)
			}
		}
	}

	return nil
}

func assertHopRecordsEqual(h1, h2 tlv.Record) error {
	if h1.Type() != h2.Type() {
		return fmt.Errorf("wrong type: expected %v, got %v", h1.Type(),
			h2.Type())
	}

	var b bytes.Buffer
	if err := h2.Encode(&b); err != nil {
		return fmt.Errorf("unable to encode record: %v", err)
	}

	if !bytes.Equal(b.Bytes(), tlvBytes) {
		return fmt.Errorf("wrong raw record: expected %x, got %x",
			tlvBytes, b.Bytes())
	}

	if h1.Size() != h2.Size() {
		return fmt.Errorf("wrong size: expected %v, "+
			"got %v", h1.Size(), h2.Size())
	}

	return nil
}

func TestRouteSerialization(t *testing.T) {
	t.Parallel()

	var b bytes.Buffer
	if err := SerializeRoute(&b, testRoute); err != nil {
		t.Fatal(err)
	}

	r := bytes.NewReader(b.Bytes())
	route2, err := DeserializeRoute(r)
	if err != nil {
		t.Fatal(err)
	}

	// First we verify all the records match up porperly, as they aren't
	// able to be properly compared using reflect.DeepEqual.
	err = assertRouteEqual(&testRoute, &route2)
	if err != nil {
		t.Fatalf("routes not equal: \n%v vs \n%v",
			spew.Sdump(testRoute), spew.Sdump(route2))
	}
}
