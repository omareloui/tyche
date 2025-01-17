package dbconn

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// DBManager handles the central users database and per-user databases
type DBManager struct {
	basePath    string
	usersDB     *sql.DB
	userDBs     map[string]*sql.DB
	userDBMutex sync.RWMutex
}

// NewDBManager creates a new database manager instance
func NewDBManager(basePath string) (*DBManager, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	// Connect to central users database
	usersDB, err := sql.Open("sqlite3", filepath.Join(basePath, "users.db?_journal_mode=WAL"))
	if err != nil {
		return nil, fmt.Errorf("failed to open users database: %w", err)
	}

	manager := &DBManager{
		basePath: basePath,
		usersDB:  usersDB,
		userDBs:  make(map[string]*sql.DB),
	}

	// Initialize central database schema
	if err := manager.initUsersDB(); err != nil {
		usersDB.Close()
		return nil, err
	}

	return manager, nil
}

// User represents a user record
type User struct {
	ID        string
	Username  string
	Email     string
	CreatedAt time.Time
	LastLogin *time.Time
}

func (m *DBManager) initUsersDB() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_login TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_databases (
			user_id TEXT PRIMARY KEY,
			db_path TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
	}

	for _, query := range queries {
		if _, err := m.usersDB.Exec(query); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}

	return nil
}

// CreateUser creates a new user and their database
func (m *DBManager) CreateUser(username, email, passwordHash string) (string, error) {
	userID := uuid.New().String()

	// Start transaction
	tx, err := m.usersDB.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert user
	_, err = tx.Exec(
		"INSERT INTO users (id, username, email, password_hash) VALUES (?, ?, ?, ?)",
		userID, username, email, passwordHash,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert user: %w", err)
	}

	// Create and register user's database
	dbPath := filepath.Join(m.basePath, fmt.Sprintf("user_%s.db", userID))
	_, err = tx.Exec(
		"INSERT INTO user_databases (user_id, db_path) VALUES (?, ?)",
		userID, dbPath,
	)
	if err != nil {
		return "", fmt.Errorf("failed to register user database: %w", err)
	}

	// Initialize user's database
	if err := m.initUserDB(dbPath); err != nil {
		return "", fmt.Errorf("failed to initialize user database: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return userID, nil
}

func (m *DBManager) initUserDB(dbPath string) error {
	// Open database with WAL mode enabled
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return fmt.Errorf("failed to open user database: %w", err)
	}
	defer db.Close()

	// Example schema - customize based on your needs
	queries := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			title TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id)
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to create user schema: %w", err)
		}
	}

	return nil
}

// GetUserDB gets a connection to a user's database
func (m *DBManager) GetUserDB(userID string) (*sql.DB, error) {
	m.userDBMutex.RLock()
	if db, exists := m.userDBs[userID]; exists {
		m.userDBMutex.RUnlock()
		return db, nil
	}
	m.userDBMutex.RUnlock()

	// Get database path
	var dbPath string
	err := m.usersDB.QueryRow(
		"SELECT db_path FROM user_databases WHERE user_id = ?",
		userID,
	).Scan(&dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get user database path: %w", err)
	}

	// Open database connection with WAL mode
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open user database: %w", err)
	}

	// Cache the connection
	m.userDBMutex.Lock()
	m.userDBs[userID] = db
	m.userDBMutex.Unlock()

	return db, nil
}

// UpdateLastLogin updates the user's last login timestamp
func (m *DBManager) UpdateLastLogin(userID string) error {
	_, err := m.usersDB.Exec(
		"UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = ?",
		userID,
	)
	return err
}

// DeleteUser deletes a user and their database
func (m *DBManager) DeleteUser(userID string) error {
	// Start transaction
	tx, err := m.usersDB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get database path
	var dbPath string
	err = tx.QueryRow(
		"SELECT db_path FROM user_databases WHERE user_id = ?",
		userID,
	).Scan(&dbPath)
	if err != nil {
		return fmt.Errorf("failed to get user database path: %w", err)
	}

	// Delete database file
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database file: %w", err)
	}

	// Remove cached connection
	m.userDBMutex.Lock()
	if db, exists := m.userDBs[userID]; exists {
		db.Close()
		delete(m.userDBs, userID)
	}
	m.userDBMutex.Unlock()

	// Delete database record and user
	_, err = tx.Exec("DELETE FROM user_databases WHERE user_id = ?", userID)
	if err != nil {
		return fmt.Errorf("failed to delete database record: %w", err)
	}

	_, err = tx.Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return tx.Commit()
}

// Close closes all database connections
func (m *DBManager) Close() error {
	m.userDBMutex.Lock()
	defer m.userDBMutex.Unlock()

	for _, db := range m.userDBs {
		db.Close()
	}
	return m.usersDB.Close()
}
