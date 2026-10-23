package store

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Mongo implementa Store sobre MongoDB.
// Todas las consultas usan filtros construidos con bson.M y valores tipados
// (nunca JSON del cliente), lo que evita inyección de operadores NoSQL.
type Mongo struct {
	client   *mongo.Client
	users    *mongo.Collection
	sessions *mongo.Collection
	audit    *mongo.Collection
}

func NewMongo(ctx context.Context, uri, dbName string) (*Mongo, error) {
	opts := options.Client().ApplyURI(uri).
		SetServerSelectionTimeout(10 * time.Second).
		SetMaxPoolSize(20)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	db := client.Database(dbName)
	m := &Mongo{
		client:   client,
		users:    db.Collection("users"),
		sessions: db.Collection("sessions"),
		audit:    db.Collection("audit_log"),
	}
	if err := m.ensureIndexes(ctx); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	return m, nil
}

func (m *Mongo) ensureIndexes(ctx context.Context) error {
	if _, err := m.users.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}
	// TTL: Mongo elimina automáticamente las sesiones vencidas.
	if _, err := m.sessions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "expires_at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
	}); err != nil {
		return err
	}
	// La auditoría se conserva 180 días.
	_, err := m.audit.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(180 * 24 * 3600),
	})
	return err
}

func notFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}

func (m *Mongo) CountUsers(ctx context.Context) (int64, error) {
	return m.users.CountDocuments(ctx, bson.M{})
}

func (m *Mongo) CreateUser(ctx context.Context, u *User) error {
	_, err := m.users.InsertOne(ctx, u)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicate
	}
	return err
}

func (m *Mongo) GetUserByID(ctx context.Context, id string) (*User, error) {
	var u User
	if err := m.users.FindOne(ctx, bson.M{"_id": id}).Decode(&u); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

func (m *Mongo) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var u User
	if err := m.users.FindOne(ctx, bson.M{"email": email}).Decode(&u); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

func (m *Mongo) ListUsers(ctx context.Context) ([]User, error) {
	cur, err := m.users.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "email", Value: 1}}).SetLimit(1000))
	if err != nil {
		return nil, err
	}
	out := []User{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *Mongo) UpdateUser(ctx context.Context, u *User) error {
	res, err := m.users.ReplaceOne(ctx, bson.M{"_id": u.ID}, u)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicate
		}
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Mongo) CreateSession(ctx context.Context, s *Session) error {
	_, err := m.sessions.InsertOne(ctx, s)
	return err
}

func (m *Mongo) GetSession(ctx context.Context, h string) (*Session, error) {
	var s Session
	if err := m.sessions.FindOne(ctx, bson.M{"_id": h}).Decode(&s); err != nil {
		return nil, notFound(err)
	}
	return &s, nil
}

func (m *Mongo) TouchSession(ctx context.Context, h string, at time.Time) error {
	_, err := m.sessions.UpdateOne(ctx, bson.M{"_id": h}, bson.M{"$set": bson.M{"last_seen_at": at}})
	return err
}

func (m *Mongo) DeleteSession(ctx context.Context, h string) error {
	_, err := m.sessions.DeleteOne(ctx, bson.M{"_id": h})
	return err
}

func (m *Mongo) DeleteUserSessions(ctx context.Context, userID string) error {
	_, err := m.sessions.DeleteMany(ctx, bson.M{"user_id": userID})
	return err
}

func (m *Mongo) AddAudit(ctx context.Context, e *AuditEntry) error {
	_, err := m.audit.InsertOne(ctx, e)
	return err
}

func (m *Mongo) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	cur, err := m.audit.Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	out := []AuditEntry{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *Mongo) Ping(ctx context.Context) error  { return m.client.Ping(ctx, nil) }
func (m *Mongo) Close(ctx context.Context) error { return m.client.Disconnect(ctx) }
