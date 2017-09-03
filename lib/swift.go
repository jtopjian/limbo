package lib

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/gophercloud/gophercloud/openstack/objectstorage/v1/swauth"
)

type SwiftAuthOpts struct {
	DomainID         string
	DomainName       string
	IdentityEndpoint string
	Password         string
	TenantID         string
	TenantName       string
	TokenID          string
	Username         string
	UserID           string
	RegionName       string
	CACert           string
	Insecure         bool
	Swauth           bool
}

func GetSwiftClient(opts SwiftAuthOpts) (*gophercloud.ServiceClient, error) {
	ao := gophercloud.AuthOptions{
		DomainID:         opts.DomainID,
		DomainName:       opts.DomainName,
		IdentityEndpoint: opts.IdentityEndpoint,
		Password:         opts.Password,
		TenantID:         opts.TenantID,
		TenantName:       opts.TenantName,
		TokenID:          opts.TokenID,
		Username:         opts.Username,
		UserID:           opts.UserID,
	}

	client, err := openstack.NewClient(ao.IdentityEndpoint)
	if err != nil {
		return nil, fmt.Errorf("Unable to create new OpenStack client: %s", err)
	}

	config := &tls.Config{}
	if v := opts.CACert; v != "" {
		caCert, err := ioutil.ReadFile(v)
		if err != nil {
			return nil, fmt.Errorf("Unable to read CA file: %s", err)
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(caCert))
		config.RootCAs = caCertPool
	}

	config.InsecureSkipVerify = opts.Insecure

	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, TLSClientConfig: config}
	client.HTTPClient.Transport = transport

	if opts.Swauth {
		return swauth.NewObjectStorageV1(client, swauth.AuthOpts{
			User: ao.Username,
			Key:  ao.Password,
		})
	}

	err = openstack.Authenticate(client, ao)
	if err != nil {
		return nil, fmt.Errorf("Unable to authenticate to OpenStack: %s", err)
	}

	return openstack.NewObjectStorageV1(client, gophercloud.EndpointOpts{
		Region: opts.RegionName,
	})
}

func SwiftCreateContainer(client *gophercloud.ServiceClient, name string, create, archive bool) error {
	archiveName := name + "_archive"
	_, err := containers.Get(client, name).Extract()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); !ok {
			return fmt.Errorf("Unable to get storage container: %s", err)
		}

		if !create {
			err = fmt.Errorf("Storage container does not exist. " +
				"Use --create-storage-container to create it")
			return err
		}

		createOpts := containers.CreateOpts{}
		if archive {
			createOpts.VersionsLocation = archiveName
		}

		_, err := containers.Create(client, name, createOpts).Extract()
		if err != nil {
			return fmt.Errorf("Unable to create storage container %s: %s", name, err)
		}

		if archive {
			if err := SwiftCreateContainer(client, archiveName, true, false); err != nil {
				return err
			}
		}

		return nil
	}

	if archive {
		updateOpts := containers.UpdateOpts{
			VersionsLocation: archiveName,
		}

		_, err := containers.Update(client, name, updateOpts).Extract()
		if err != nil {
			return fmt.Errorf("Unable to enable archiving: %s", err)
		}

		if err := SwiftCreateContainer(client, archiveName, true, false); err != nil {
			return err
		}
	}

	return nil
}

type SwiftUploadOpts struct {
	SourceName       string
	StorageContainer string
	ObjectName       string
}

type SwiftUploadResults struct {
	Headers *objects.CreateHeader
}

func SwiftUploadObject(client *gophercloud.ServiceClient, opts SwiftUploadOpts) (*SwiftUploadResults, error) {
	f, err := os.Open(opts.SourceName)
	if err != nil {
		return nil, fmt.Errorf("Unable to open file: %s", err)
	}
	defer f.Close()

	createOpts := objects.CreateOpts{
		Content: f,
	}

	result, err := objects.Create(client, opts.StorageContainer, opts.ObjectName, createOpts).Extract()
	if err != nil {
		return nil, fmt.Errorf("Unable to upload file to swift: %s", err)
	}

	s := &SwiftUploadResults{
		Headers: result,
	}

	return s, nil
}

type SwiftDownloadOpts struct {
	Filename         string
	ObjectName       string
	StorageContainer string
}

type SwiftDownloadResult struct {
	Headers *objects.DownloadHeader
}

func SwiftDownloadObject(client *gophercloud.ServiceClient, opts SwiftDownloadOpts) (*SwiftDownloadResult, error) {
	_, err := objects.Get(client, opts.StorageContainer, opts.ObjectName, nil).Extract()
	if err != nil {
		if _, ok := err.(gophercloud.ErrDefault404); ok {
			return nil, ErrObjectDoesNotExist{}
		}

		return nil, err
	}

	object := objects.Download(client, opts.StorageContainer, opts.ObjectName, nil)
	f, err := object.ExtractContent()
	if err != nil {
		return nil, fmt.Errorf("Unable to download object: %s", err)
	}

	err = ioutil.WriteFile(opts.Filename, f, 0640)
	if err != nil {
		return nil, fmt.Errorf("Unable to save object: %s", err)
	}

	downloadHeaders, err := object.Extract()
	if err != nil {
		return nil, fmt.Errorf("Unable to extract download headers: %s", err)
	}

	result := &SwiftDownloadResult{
		Headers: downloadHeaders,
	}

	return result, nil
}
