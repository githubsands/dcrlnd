package channeldb

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrlnd/lntypes"
)

// TestWitnessCacheRetrieval tests that we're able to add and lookup new
// witnesses to the witness cache.
func TestWitnessCacheRetrieval(t *testing.T) {
	t.Parallel()

	cdb, cleanUp, err := makeTestDB()
	if err != nil {
		t.Fatalf("unable to make test database: %v", err)
	}
	defer cleanUp()

	wCache := cdb.NewWitnessCache()

	// We'll be attempting to add then lookup two simple hash witnesses
	// within this test.
	witness1 := rev[:]
	witness1Key := chainhash.HashB(witness1)

	witness2 := key[:]
	witness2Key := chainhash.HashB(witness2)

	witnesses := [][]byte{witness1, witness2}
	keys := [][]byte{witness1Key[:], witness2Key[:]}

	// First, we'll attempt to add the witnesses to the database.
	err = wCache.AddWitnesses(Sha256HashWitness, witnesses...)
	if err != nil {
		t.Fatalf("unable to add witness: %v", err)
	}

	// With the witnesses stored, we'll now attempt to look them up.
	for i, key := range keys {
		witness := witnesses[i]

		// We should get back the *exact* same witness as we originally
		// stored.
		dbWitness, err := wCache.LookupWitness(Sha256HashWitness, key)
		if err != nil {
			t.Fatalf("unable to look up witness: %v", err)
		}

		if !reflect.DeepEqual(witness, dbWitness[:]) {
			t.Fatalf("witnesses don't match: expected %x, got %x",
				witness[:], dbWitness[:])
		}
	}
}

// TestWitnessCacheDeletion tests that we're able to delete a single witness,
// and also a class of witnesses from the cache.
func TestWitnessCacheDeletion(t *testing.T) {
	t.Parallel()

	cdb, cleanUp, err := makeTestDB()
	if err != nil {
		t.Fatalf("unable to make test database: %v", err)
	}
	defer cleanUp()

	wCache := cdb.NewWitnessCache()

	// We'll start by adding two witnesses to the cache.
	witness1 := rev[:]
	witness1Key := chainhash.HashB(witness1)

	if err := wCache.AddWitnesses(Sha256HashWitness, witness1); err != nil {
		t.Fatalf("unable to add witness: %v", err)
	}

	witness2 := key[:]
	witness2Key := chainhash.HashB(witness2)

	if err := wCache.AddWitnesses(Sha256HashWitness, witness2); err != nil {
		t.Fatalf("unable to add witness: %v", err)
	}

	// We'll now delete the first witness. If we attempt to look it up, we
	// should get ErrNoWitnesses.
	err = wCache.DeleteWitness(Sha256HashWitness, witness1Key[:])
	if err != nil {
		t.Fatalf("unable to delete witness: %v", err)
	}
	_, err = wCache.LookupWitness(Sha256HashWitness, witness1Key[:])
	if err != ErrNoWitnesses {
		t.Fatalf("expected ErrNoWitnesses instead got: %v", err)
	}

	// Next, we'll attempt to delete the entire witness class itself. When
	// we try to lookup the second witness, we should again get
	// ErrNoWitnesses.
	if err := wCache.DeleteWitnessClass(Sha256HashWitness); err != nil {
		t.Fatalf("unable to delete witness class: %v", err)
	}
	_, err = wCache.LookupWitness(Sha256HashWitness, witness2Key[:])
	if err != ErrNoWitnesses {
		t.Fatalf("expected ErrNoWitnesses instead got: %v", err)
	}
}

// TestWitnessCacheUnknownWitness tests that we get an error if we attempt to
// query/add/delete an unknown witness.
func TestWitnessCacheUnknownWitness(t *testing.T) {
	t.Parallel()

	cdb, cleanUp, err := makeTestDB()
	if err != nil {
		t.Fatalf("unable to make test database: %v", err)
	}
	defer cleanUp()

	wCache := cdb.NewWitnessCache()

	// We'll attempt to add a new, undefined witness type to the database.
	// We should get an error.
	err = wCache.AddWitnesses(234, key[:])
	if err != ErrUnknownWitnessType {
		t.Fatalf("expected ErrUnknownWitnessType, got %v", err)
	}
}

// TestAddSha256Witnesses tests that insertion using AddSha256Witnesses behaves
// identically to the insertion via the generalized interface.
func TestAddSha256Witnesses(t *testing.T) {
	cdb, cleanUp, err := makeTestDB()
	if err != nil {
		t.Fatalf("unable to make test database: %v", err)
	}
	defer cleanUp()

	wCache := cdb.NewWitnessCache()

	// We'll start by adding a witnesses to the cache using the generic
	// AddWitnesses method.
	witness1 := rev[:]
	witness1Key := chainhash.HashB(witness1)

	witness2 := key[:]
	witness2Key := chainhash.HashB(witness2)

	var (
		preimages = []lntypes.Preimage{rev, key}
		witnesses = [][]byte{witness1, witness2}
		keys      = [][]byte{witness1Key[:], witness2Key[:]}
	)

	err = wCache.AddWitnesses(Sha256HashWitness, witnesses...)
	if err != nil {
		t.Fatalf("unable to add witness: %v", err)
	}

	for i, key := range keys {
		witness := witnesses[i]

		dbWitness, err := wCache.LookupWitness(
			Sha256HashWitness, key,
		)
		if err != nil {
			t.Fatalf("unable to lookup witness: %v", err)
		}

		// Assert that the retrieved witness matches the original.
		if bytes.Compare(dbWitness, witness) != 0 {
			t.Fatalf("retrieved witness mismatch, want: %x, "+
				"got: %x", witness, dbWitness)
		}

		// We'll now delete the witness, as we'll be reinserting it
		// using the specialized AddSha256Witnesses method.
		err = wCache.DeleteWitness(Sha256HashWitness, key)
		if err != nil {
			t.Fatalf("unable to delete witness: %v", err)
		}
	}

	// Now, add the same witnesses using the type-safe interface for
	// lntypes.Preimages..
	err = wCache.AddSha256Witnesses(preimages...)
	if err != nil {
		t.Fatalf("unable to add chainhash preimage: %v", err)
	}

	// Finally, iterate over the keys and assert that the returned witnesses
	// match the original witnesses. This asserts that the specialized
	// insertion method behaves identically to the generalized interface.
	for i, key := range keys {
		witness := witnesses[i]

		dbWitness, err := wCache.LookupWitness(
			Sha256HashWitness, key,
		)
		if err != nil {
			t.Fatalf("unable to lookup witness: %v", err)
		}

		// Assert that the retrieved witness matches the original.
		if bytes.Compare(dbWitness, witness) != 0 {
			t.Fatalf("retrieved witness mismatch, want: %x, "+
				"got: %x", witness, dbWitness)
		}
	}
}
