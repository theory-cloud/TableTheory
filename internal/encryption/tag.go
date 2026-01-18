package encryption

import (
	"fmt"

	customerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/session"
)

func MetadataHasEncryptedFields(metadata *model.Metadata) bool {
	if metadata == nil {
		return false
	}
	for _, fieldMeta := range metadata.Fields {
		if fieldMeta == nil {
			continue
		}
		if fieldMeta.IsEncrypted {
			return true
		}
		if _, ok := fieldMeta.Tags["encrypted"]; ok {
			return true
		}
	}
	return false
}

func FailClosedIfEncryptedWithoutKMSKeyARN(sess *session.Session, metadata *model.Metadata) error {
	if metadata == nil || !MetadataHasEncryptedFields(metadata) {
		return nil
	}

	keyARN := ""
	if sess != nil && sess.Config() != nil {
		keyARN = sess.Config().KMSKeyARN
	}
	if keyARN != "" {
		return nil
	}

	return fmt.Errorf("%w: model %s contains theorydb:\"encrypted\" fields but session.Config.KMSKeyARN is empty", customerrors.ErrEncryptionNotConfigured, metadata.Type.Name())
}
