package instance

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/cozy/cozy-stack/apps"
	"github.com/cozy/cozy-stack/config"
	"github.com/cozy/cozy-stack/couchdb"
	"github.com/cozy/cozy-stack/couchdb/mango"
	"github.com/cozy/cozy-stack/vfs"
	"github.com/dchest/uniuri"
	scrypt "github.com/elithrar/simple-scrypt"
	"github.com/spf13/afero"
)

// InstanceType : The couchdb type for an Instance
const InstanceType = "instances"

var (
	// ErrNotFound is used when the seeked instance was not found
	ErrNotFound = errors.New("Instance not found")
	// ErrExists is used the instance already exists
	ErrExists = errors.New("Instance already exists")
	// ErrIllegalDomain is used when the domain named contains illegal characters
	ErrIllegalDomain = errors.New("Domain name contains illegal characters")
)

// An Instance has the informations relatives to the logical cozy instance,
// like the domain, the locale or the access to the databases and files storage
// It is a couchdb.Doc to be persisted in couchdb.
type Instance struct {
	DocID         string `json:"_id,omitempty"`  // couchdb _id
	DocRev        string `json:"_rev,omitempty"` // couchdb _rev
	Domain        string `json:"domain"`         // The main DNS domain, like example.cozycloud.cc
	StorageURL    string `json:"storage"`        // Where the binaries are persisted
	RegisterToken string `json:"registerToken,omitempty"`
	PasswordHash  string `json:"passwordHash,omitempty"`
	storage       afero.Fs
}

// DocType implements couchdb.Doc
func (i *Instance) DocType() string { return InstanceType }

// ID implements couchdb.Doc
func (i *Instance) ID() string { return i.DocID }

// SetID implements couchdb.Doc
func (i *Instance) SetID(v string) { i.DocID = v }

// Rev implements couchdb.Doc
func (i *Instance) Rev() string { return i.DocRev }

// SetRev implements couchdb.Doc
func (i *Instance) SetRev(v string) { i.DocRev = v }

// SubDomain returns the full url for a subdomain of this instance
// usefull with apps slugs
// TODO https is hardcoded
func (i *Instance) SubDomain(s string) string {
	return "https://" + s + "." + i.Domain
}

// ensure Instance implements couchdb.Doc & vfs.Context
var _ couchdb.Doc = (*Instance)(nil)
var _ vfs.Context = (*Instance)(nil)

// CreateInCouchdb create the instance doc in the global database
func (i *Instance) createInCouchdb() (err error) {
	if _, err = Get(i.Domain); err == nil {
		return ErrExists
	}
	if err != nil && err != ErrNotFound {
		return err
	}
	err = couchdb.CreateDoc(couchdb.GlobalDB, i)
	if err != nil {
		return err
	}
	byDomain := mango.IndexOnFields("domain")
	return couchdb.DefineIndex(couchdb.GlobalDB, InstanceType, byDomain)
}

// createRootFolder creates the root folder for this instance
func (i *Instance) createRootFolder() error {
	rootFsURL := config.BuildAbsFsURL("/")
	domainURL := config.BuildRelFsURL(i.Domain)

	rootFs, err := createFs(rootFsURL)
	if err != nil {
		return err
	}

	if err = rootFs.MkdirAll(domainURL.Path, 0755); err != nil {
		return err
	}

	if err = vfs.CreateRootDirDoc(i); err != nil {
		rootFs.Remove(domainURL.Path)
		return err
	}

	return nil
}

// createFSIndexes creates the index needed by VFS
func (i *Instance) createFSIndexes() error {
	for _, index := range vfs.Indexes {
		err := couchdb.DefineIndex(i, vfs.FsDocType, index)
		if err != nil {
			return err
		}
	}
	return nil
}

// createAppsDB creates the database needed for Apps
func (i *Instance) createAppsDB() error {
	return couchdb.CreateDB(i, apps.ManifestDocType)
}

// Create build an instance and .Create it
func Create(domain string, locale string, apps []string) (*Instance, error) {
	if strings.ContainsAny(domain, vfs.ForbiddenFilenameChars) || domain == ".." || domain == "." {
		return nil, ErrIllegalDomain
	}

	domainURL := config.BuildRelFsURL(domain)
	i := &Instance{
		Domain:        domain,
		StorageURL:    domainURL.String(),
		RegisterToken: uniuri.New(),
	}

	var err error

	err = i.makeStorageFs()
	if err != nil {
		return nil, err
	}

	err = i.createInCouchdb()
	if err != nil {
		return nil, err
	}

	err = i.createRootFolder()
	if err != nil {
		return nil, err
	}

	err = i.createFSIndexes()
	if err != nil {
		return nil, err
	}

	err = i.createAppsDB()
	if err != nil {
		return nil, err
	}

	// TODO atomicity with defer
	// TODO figure out what to do with locale
	// TODO install apps

	return i, nil
}

func (i *Instance) makeStorageFs() error {
	u, err := url.Parse(i.StorageURL)
	if err != nil {
		return err
	}
	i.storage, err = createFs(u)
	return err
}

// Get retrieves the instance for a request by its host.
func Get(domain string) (*Instance, error) {
	if config.IsDevRelease() {
		if domain == "" || strings.Contains(domain, "127.0.0.1") || strings.Contains(domain, "localhost") {
			domain = "dev"
		}
	}

	var instances []*Instance
	req := &couchdb.FindRequest{
		Selector: mango.Equal("domain", domain),
		Limit:    1,
	}
	err := couchdb.FindDocs(couchdb.GlobalDB, InstanceType, req, &instances)
	if couchdb.IsNoDatabaseError(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, ErrNotFound
	}

	err = instances[0].makeStorageFs()
	if err != nil {
		return nil, err
	}

	return instances[0], nil
}

// List returns the list of declared instances.
//
// TODO: pagination
// TODO: don't return the design docs
func List() ([]*Instance, error) {
	var docs []*Instance
	req := &couchdb.AllDocsRequest{Limit: 100}
	err := couchdb.GetAllDocs(couchdb.GlobalDB, InstanceType, req, &docs)
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// Destroy is used to remove the instance. All the data linked to this
// instance will be permanently deleted.
func Destroy(domain string) (*Instance, error) {
	i, err := Get(domain)
	if err != nil {
		return nil, err
	}

	if err = couchdb.DeleteDoc(couchdb.GlobalDB, i); err != nil {
		return nil, err
	}

	if err = couchdb.DeleteAllDBs(i); err != nil {
		return nil, err
	}

	rootFsURL := config.BuildAbsFsURL("/")
	domainURL := config.BuildRelFsURL(i.Domain)

	rootFs, err := createFs(rootFsURL)
	if err != nil {
		return nil, err
	}

	if err = rootFs.RemoveAll(domainURL.Path); err != nil {
		return nil, err
	}

	return i, nil
}

// FS returns the afero storage provider where the binaries for
// the current instance are persisted
func (i *Instance) FS() afero.Fs {
	if i.storage == nil {
		if err := i.makeStorageFs(); err != nil {
			panic(err)
		}
	}
	return i.storage
}

// Prefix returns the prefix to use in database naming for the
// current instance
func (i *Instance) Prefix() string {
	return i.Domain + "/"
}

// RegisterPassword replace the instance registerToken by a password
func (i *Instance) RegisterPassword(pass string, tok string) error {
	if i.RegisterToken == "" {
		return errors.New("Missing registerToken")
	}
	if i.RegisterToken != tok {
		return errors.New("Invalid registerToken")
	}

	hash, err := scrypt.GenerateFromPassword([]byte(pass), scrypt.DefaultParams)
	if err != nil {
		return err
	}

	i.RegisterToken = ""
	i.PasswordHash = string(hash)

	return couchdb.UpdateDoc(couchdb.GlobalDB, i)
}

// CheckPassword confirm an instance passport
func (i *Instance) CheckPassword(pass string) error {
	hash := []byte(i.PasswordHash)
	return scrypt.CompareHashAndPassword(hash, []byte(pass))
}

func createFs(u *url.URL) (fs afero.Fs, err error) {
	switch u.Scheme {
	case "file":
		fs = afero.NewBasePathFs(afero.NewOsFs(), u.Path)
	case "mem":
		fs = afero.NewMemMapFs()
	default:
		err = fmt.Errorf("Unknown storage provider: %v", u.Scheme)
	}
	return
}
