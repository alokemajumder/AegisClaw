package evidence

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/alokemajumder/AegisClaw/internal/config"
)

// Store provides evidence artifact storage backed by MinIO.
type Store struct {
	client *minio.Client
	bucket string
	logger *slog.Logger
}

// Artifact represents a stored evidence artifact.
type Artifact struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	StepID      string    `json:"step_id,omitempty"`
	Name        string    `json:"name"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	StoredAt    time.Time `json:"stored_at"`
}

// NewStore creates a new evidence store backed by MinIO.
func NewStore(ctx context.Context, cfg config.MinIOConfig, logger *slog.Logger) (*Store, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("creating minio client: %w", err)
	}

	// Ensure bucket exists
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("checking bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("creating bucket %s: %w", cfg.Bucket, err)
		}
		logger.Info("evidence bucket created", "bucket", cfg.Bucket)
	}

	logger.Info("evidence store initialized", "endpoint", cfg.Endpoint, "bucket", cfg.Bucket)

	return &Store{
		client: client,
		bucket: cfg.Bucket,
		logger: logger,
	}, nil
}

// Upload stores an evidence artifact.
func (s *Store) Upload(ctx context.Context, runID, name, contentType string, data []byte) (*Artifact, error) {
	id := fmt.Sprintf("ev_%s", uuid.New().String()[:12])
	objectName := fmt.Sprintf("%s/%s/%s", runID, id, name)

	reader := bytes.NewReader(data)
	info, err := s.client.PutObject(ctx, s.bucket, objectName, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: map[string]string{
			"evidence-id": id,
			"run-id":      runID,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("uploading evidence: %w", err)
	}

	s.logger.Debug("evidence uploaded", "id", id, "object", objectName, "size", info.Size)

	return &Artifact{
		ID:          id,
		RunID:       runID,
		Name:        name,
		ContentType: contentType,
		Size:        info.Size,
		StoredAt:    time.Now().UTC(),
	}, nil
}

// Download retrieves an evidence artifact.
func (s *Store) Download(ctx context.Context, runID, evidenceID, name string) ([]byte, error) {
	objectName := fmt.Sprintf("%s/%s/%s", runID, evidenceID, name)

	obj, err := s.client.GetObject(ctx, s.bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting evidence object: %w", err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("reading evidence object: %w", err)
	}

	return data, nil
}

// List returns all evidence artifacts for a run.
func (s *Store) List(ctx context.Context, runID string) ([]Artifact, error) {
	prefix := fmt.Sprintf("%s/", runID)
	var artifacts []Artifact

	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("listing evidence: %w", obj.Err)
		}
		artifacts = append(artifacts, Artifact{
			RunID:    runID,
			Name:     obj.Key,
			Size:     obj.Size,
			StoredAt: obj.LastModified,
		})
	}

	return artifacts, nil
}

// HealthCheck verifies the MinIO connection.
func (s *Store) HealthCheck(ctx context.Context) error {
	_, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("evidence store health check: %w", err)
	}
	return nil
}

// UploadReceipt stores an immutable run receipt.
func (s *Store) UploadReceipt(ctx context.Context, runID string, receiptData []byte) error {
	objectName := fmt.Sprintf("receipts/%s.json", runID)

	reader := bytes.NewReader(receiptData)
	_, err := s.client.PutObject(ctx, s.bucket, objectName, reader, int64(len(receiptData)), minio.PutObjectOptions{
		ContentType: "application/json",
		UserMetadata: map[string]string{
			"type":   "run-receipt",
			"run-id": runID,
		},
	})
	if err != nil {
		return fmt.Errorf("uploading receipt: %w", err)
	}

	return nil
}
