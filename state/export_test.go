// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/juju/juju/testcharms"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	"github.com/juju/utils/filestorage"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/backups"
)

const (
	UnitsC       = unitsC
	ServicesC    = servicesC
	SettingsC    = settingsC
	BackupsMetaC = backupsMetaC
)

var (
	NewMongoConnInfo = newMongoConnInfo

	GetManagedStorage     = (*State).getManagedStorage
	ToolstorageNewStorage = &toolstorageNewStorage
)

var _ filestorage.DocStorage = (*backupsDocStorage)(nil)
var _ filestorage.RawFileStorage = (*backupBlobStorage)(nil)

func newBackupDoc(meta *backups.Metadata) *backupMetaDoc {
	var doc backupMetaDoc
	doc.UpdateFromMetadata(meta)
	return &doc
}

func getBackupDBWrapper(st *State) *backupDBWrapper {
	envUUID := st.EnvironTag().Id()
	db := st.db.Session.DB(backupDB)
	return newBackupDBWrapper(db, BackupsMetaC, envUUID)
}

// NewBackupID creates a new backup ID based on the metadata.
func NewBackupID(meta *backups.Metadata) string {
	doc := newBackupDoc(meta)
	return newBackupID(doc)
}

// GetBackupMetadata returns the metadata retrieved from storage.
func GetBackupMetadata(st *State, id string) (*backups.Metadata, error) {
	db := getBackupDBWrapper(st)
	defer db.Close()
	doc, err := getBackupMetadata(db, id)
	if err != nil {
		return nil, err
	}
	return doc.asMetadata(), nil
}

// AddBackupMetadata adds the metadata to storage.
func AddBackupMetadata(st *State, meta *backups.Metadata) (string, error) {
	db := getBackupDBWrapper(st)
	defer db.Close()
	doc := newBackupDoc(meta)
	return addBackupMetadata(db, doc)
}

// AddBackupMetadataID adds the metadata to storage, using the given
// backup ID.
func AddBackupMetadataID(st *State, meta *backups.Metadata, id string) error {
	db := getBackupDBWrapper(st)
	defer db.Close()
	doc := newBackupDoc(meta)
	return addBackupMetadataID(db, doc, id)
}

// SetBackupStoredTime stores the time of when the identified backup archive
// file was stored.
func SetBackupStoredTime(st *State, id string, stored time.Time) error {
	db := getBackupDBWrapper(st)
	defer db.Close()
	return setBackupStoredTime(db, id, stored)
}

func SetTestHooks(c *gc.C, st *State, hooks ...jujutxn.TestHook) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetTestHooks(c, runner, hooks...)
}

func SetBeforeHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetBeforeHooks(c, runner, fs...)
}

func SetAfterHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetAfterHooks(c, runner, fs...)
}

func SetRetryHooks(c *gc.C, st *State, block, check func()) txntesting.TransactionChecker {
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: st.db})
	st.transactionRunner = runner
	return txntesting.SetRetryHooks(c, runner, block, check)
}

// SetPolicy updates the State's policy field to the
// given Policy, and returns the old value.
func SetPolicy(st *State, p Policy) Policy {
	old := st.policy
	st.policy = p
	return old
}

type (
	CharmDoc    charmDoc
	MachineDoc  machineDoc
	RelationDoc relationDoc
	ServiceDoc  serviceDoc
	UnitDoc     unitDoc
)

func (doc *MachineDoc) String() string {
	m := &Machine{doc: machineDoc(*doc)}
	return m.String()
}

func ServiceSettingsRefCount(st *State, serviceName string, curl *charm.URL) (int, error) {
	settingsRefsCollection, closer := st.getCollection(settingsrefsC)
	defer closer()

	key := serviceSettingsKey(serviceName, curl)
	var doc settingsRefsDoc
	if err := settingsRefsCollection.FindId(st.docID(key)).One(&doc); err == nil {
		return doc.RefCount, nil
	}
	return 0, mgo.ErrNotFound
}

func AddTestingCharm(c *gc.C, st *State, name string) *Charm {
	return addCharm(c, st, "quantal", testcharms.Repo.CharmDir(name))
}

func AddTestingService(c *gc.C, st *State, name string, ch *Charm, owner names.UserTag) *Service {
	c.Assert(ch, gc.NotNil)
	return AddTestingServiceWithNetworks(c, st, name, ch, owner, nil)
}

func AddTestingServiceWithNetworks(c *gc.C, st *State, name string, ch *Charm, owner names.UserTag, networks []string) *Service {
	c.Assert(ch, gc.NotNil)
	service, err := st.AddService(name, owner.String(), ch, networks)
	c.Assert(err, gc.IsNil)
	return service
}

func AddCustomCharm(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	path := testcharms.Repo.ClonedDirPath(c.MkDir(), name)
	if filename != "" {
		config := filepath.Join(path, filename)
		err := ioutil.WriteFile(config, []byte(content), 0644)
		c.Assert(err, gc.IsNil)
	}
	ch, err := charm.ReadCharmDir(path)
	c.Assert(err, gc.IsNil)
	if revision != -1 {
		ch.SetRevision(revision)
	}
	return addCharm(c, st, series, ch)
}

func addCharm(c *gc.C, st *State, series string, ch charm.Charm) *Charm {
	ident := fmt.Sprintf("%s-%s-%d", series, ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("local:" + series + "/" + ident)
	sch, err := st.AddCharm(ch, curl, "dummy-path", ident+"-sha256")
	c.Assert(err, gc.IsNil)
	return sch
}

// SetCharmBundleURL sets the deprecated bundleurl field in the
// charm document for the charm with the specified URL.
func SetCharmBundleURL(c *gc.C, st *State, curl *charm.URL, bundleURL string) {
	ops := []txn.Op{{
		C:      charmsC,
		Id:     st.docID(curl.String()),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"bundleurl", bundleURL}}}},
	}}
	err := st.runTransaction(ops)
	c.Assert(err, gc.IsNil)
}

var MachineIdLessThan = machineIdLessThan

// SCHEMACHANGE
// This method is used to reset the ownertag attribute
func SetServiceOwnerTag(s *Service, ownerTag string) {
	s.doc.OwnerTag = ownerTag
}

// SCHEMACHANGE
// Get the owner directly
func GetServiceOwnerTag(s *Service) string {
	return s.doc.OwnerTag
}

func SetPasswordHash(e Authenticator, passwordHash string) error {
	type hasSetPasswordHash interface {
		setPasswordHash(string) error
	}
	return e.(hasSetPasswordHash).setPasswordHash(passwordHash)
}

// Return the underlying PasswordHash stored in the database. Used by the test
// suite to check that the PasswordHash gets properly updated to new values
// when compatibility mode is detected.
func GetPasswordHash(e Authenticator) string {
	type hasGetPasswordHash interface {
		getPasswordHash() string
	}
	return e.(hasGetPasswordHash).getPasswordHash()
}

func init() {
	logSize = logSizeTests
}

// TxnRevno returns the txn-revno field of the document
// associated with the given Id in the given collection.
func TxnRevno(st *State, coll string, id interface{}) (int64, error) {
	var doc struct {
		TxnRevno int64 `bson:"txn-revno"`
	}
	err := st.db.C(coll).FindId(id).One(&doc)
	if err != nil {
		return 0, err
	}
	return doc.TxnRevno, nil
}

// MinUnitsRevno returns the Revno of the minUnits document
// associated with the given service name.
func MinUnitsRevno(st *State, serviceName string) (int, error) {
	minUnitsCollection, closer := st.getCollection(minUnitsC)
	defer closer()
	var doc minUnitsDoc
	if err := minUnitsCollection.FindId(st.docID(serviceName)).One(&doc); err != nil {
		return 0, err
	}
	return doc.Revno, nil
}

func ParseTag(st *State, tag names.Tag) (string, interface{}, error) {
	return st.parseTag(tag)
}

func RunTransaction(st *State, ops []txn.Op) error {
	return st.runTransaction(ops)
}

// Return the PasswordSalt that goes along with the PasswordHash
func GetUserPasswordSaltAndHash(u *User) (string, string) {
	return u.doc.PasswordSalt, u.doc.PasswordHash
}

var NewAddress = newAddress

func CheckUserExists(st *State, name string) (bool, error) {
	return st.checkUserExists(name)
}

var StateServerAvailable = &stateServerAvailable

func WatcherMergeIds(st *State, changeset *[]string, updates map[interface{}]bool) error {
	return mergeIds(st, changeset, updates)
}

func WatcherEnsureSuffixFn(marker string) func(string) string {
	return ensureSuffixFn(marker)
}

func WatcherMakeIdFilter(st *State, marker string, receivers ...ActionReceiver) func(interface{}) bool {
	return makeIdFilter(st, marker, receivers...)
}

var (
	GetOrCreatePorts = getOrCreatePorts
	GetPorts         = getPorts
	PortsGlobalKey   = portsGlobalKey
	NowToTheSecond   = nowToTheSecond
)

var CurrentUpgradeId = currentUpgradeId

func GetAllUpgradeInfos(st *State) ([]*UpgradeInfo, error) {
	upgradeInfos, closer := st.getCollection(upgradeInfoC)
	defer closer()

	var out []*UpgradeInfo
	var doc upgradeInfoDoc
	iter := upgradeInfos.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		out = append(out, &UpgradeInfo{st: st, doc: doc})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func DocID(st *State, id string) string {
	return st.docID(id)
}

func LocalID(st *State, id string) string {
	return st.localID(id)
}

func StrictLocalID(st *State, id string) (string, error) {
	return st.strictLocalID(id)
}

func GetUnitEnvUUID(unit *Unit) string {
	return unit.doc.EnvUUID
}
