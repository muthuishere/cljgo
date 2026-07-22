package vault

import (
	"context"
	"fmt"
	"net/url"
)

// This file holds STUB cloud providers: interface-only, not dialed. Each
// proves two things without any cloud creds:
//  1. its scheme resolves through the registry to the right provider, and
//  2. the Provider.Get shape FITS the real SDK call — the cited method returns
//     exactly a (value, notFound, err) triad, which is our (Secret, ok, err).
//
// errNotDialed is what a stub returns from Get: it is an err (backend "failure"),
// which correctly means a chain does NOT silently fall past an undialed cloud
// to a weaker source — you must configure it or remove it from the chain.
var errNotDialed = fmt.Errorf("stub provider: not dialed (no cloud creds wired in spike)")

// --- AWS Secrets Manager --- scheme: aws-sm://<secret-id>[?region=…]
//
// Real call (aws-sdk-go-v2, PURE GO, CGO_ENABLED=0):
//
//	import "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
//	out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
//	    SecretId: aws.String(p.secretID),
//	})
//	// out.SecretString  → Secret; ResourceNotFoundException → ok=false; other err → err
//
// The shape fits exactly: one string in (SecretId), one string out
// (SecretString), a typed NotFound error to map to ok=false.
type awsSMProvider struct {
	secretID string
	region   string
}

func init() {
	Register("aws-sm", func(u *url.URL) (Provider, error) {
		return awsSMProvider{secretID: u.Host + u.Path, region: u.Query().Get("region")}, nil
	})
}
func (p awsSMProvider) Name() string { return "aws-sm:" + p.secretID }
func (p awsSMProvider) Get(ctx context.Context, key string) (Secret, bool, error) {
	// Dial path would go here (secretsmanager.NewFromConfig(cfg).GetSecretValue).
	return Secret{}, false, errNotDialed
}

// --- GCP Secret Manager --- scheme: gcp-sm://projects/<p>/secrets/<s>/versions/latest
//
// Real call (cloud.google.com/go/secretmanager, PURE GO):
//
//	import secretmanager "cloud.google.com/go/secretmanager/apiv1"
//	import smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
//	res, err := c.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{Name: p.name})
//	// res.Payload.Data ([]byte) → Secret; codes.NotFound → ok=false
type gcpSMProvider struct{ name string }

func init() {
	Register("gcp-sm", func(u *url.URL) (Provider, error) {
		return gcpSMProvider{name: u.Host + u.Path}, nil
	})
}
func (p gcpSMProvider) Name() string { return "gcp-sm:" + p.name }
func (p gcpSMProvider) Get(ctx context.Context, key string) (Secret, bool, error) {
	return Secret{}, false, errNotDialed
}

// --- HashiCorp Vault --- scheme: vault://<mount>/<path>#<field>
//
// Real call (github.com/hashicorp/vault-client-go OR api, PURE GO):
//
//	import vault "github.com/hashicorp/vault-client-go"
//	s, err := c.Secrets.KvV2Read(ctx, p.path, vault.WithMountPath(p.mount))
//	// s.Data.Data[p.field] (any) → Secret; 404 → ok=false
type vaultHCProvider struct {
	mount, path, field string
}

func init() {
	Register("vault", func(u *url.URL) (Provider, error) {
		return vaultHCProvider{mount: u.Host, path: trimSlash(u.Path), field: u.Fragment}, nil
	})
}
func (p vaultHCProvider) Name() string { return "vault:" + p.mount + "/" + p.path }
func (p vaultHCProvider) Get(ctx context.Context, key string) (Secret, bool, error) {
	return Secret{}, false, errNotDialed
}

// --- GitHub Actions secrets --- scheme: gh://<owner>/<repo>#<SECRET_NAME>
//
// NOTE — an honest cljgo finding: GitHub Actions secrets are WRITE-ONLY via the
// REST API. `google/go-github`'s ActionsService has CreateOrUpdateRepoSecret
// (PUT /repos/{o}/{r}/actions/secrets/{name}) and GetRepoSecret — but GetRepoSecret
// returns only metadata (name, timestamps), NEVER the value; values are readable
// only inside a running workflow via the ${{ secrets.NAME }} env injection.
// So `gh://` fits our interface for WRITE (seal) and for the in-workflow READ
// path (which is really env:// under the hood), but a general out-of-workflow
// Get can't return the value. The stub records that boundary.
type ghActionsProvider struct {
	owner, repo, name string
}

func init() {
	Register("gh", func(u *url.URL) (Provider, error) {
		return ghActionsProvider{owner: u.Host, repo: trimSlash(u.Path), name: u.Fragment}, nil
	})
}
func (p ghActionsProvider) Name() string { return "gh:" + p.owner + "/" + p.repo }
func (p ghActionsProvider) Get(ctx context.Context, key string) (Secret, bool, error) {
	return Secret{}, false, fmt.Errorf("gh: GitHub Actions secrets are write-only out-of-workflow; read via env:// inside the workflow")
}
