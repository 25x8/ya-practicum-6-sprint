package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/25x8/ya-practicum-6-sprint/internal/models"
	_ "github.com/jackc/pgx/v4/stdlib"
)

type Repository interface {
	CreateUser(ctx context.Context, login, passwordHash string) (int64, error)
	GetUserByLogin(ctx context.Context, login string) (*models.User, error)
	GetUserByID(ctx context.Context, id int64) (*models.User, error)

	CreateOrder(ctx context.Context, userID int64, orderNumber string) error
	GetOrderByNumber(ctx context.Context, orderNumber string) (*models.Order, error)
	GetUserOrders(ctx context.Context, userID int64) ([]models.Order, error)
	UpdateOrderStatus(ctx context.Context, orderNumber, status string, accrual float64) error

	GetUserBalance(ctx context.Context, userID int64) (*models.Balance, error)
	WithdrawBalance(ctx context.Context, userID int64, orderNumber string, amount float64) error
	GetUserWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error)

	InitDB(databaseURI string) error
	Close() error
}

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(databaseURI string) *PostgresRepository {
	return &PostgresRepository{
		db: nil,
	}
}

func (r *PostgresRepository) InitDB(databaseURI string) error {
	db, err := sql.Open("pgx", databaseURI)
	if err != nil {
		return err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}

	r.db = db

	err = r.createTables()
	if err != nil {
		db.Close()
		return err
	}

	return nil
}

func (r *PostgresRepository) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

func (r *PostgresRepository) createTables() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			login VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS orders (
			id SERIAL PRIMARY KEY,
			number VARCHAR(255) UNIQUE NOT NULL,
			user_id INTEGER REFERENCES users(id),
			status VARCHAR(50) NOT NULL DEFAULT 'NEW',
			accrual NUMERIC(10, 2) DEFAULT 0,
			uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(`
		CREATE TABLE IF NOT EXISTS withdrawals (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id),
			order_number VARCHAR(255) NOT NULL,
			sum NUMERIC(10, 2) NOT NULL,
			processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	return nil
}

func (r *PostgresRepository) CreateUser(ctx context.Context, login, passwordHash string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(
		ctx,
		"INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id",
		login, passwordHash,
	).Scan(&id)

	if err != nil {
		return 0, err
	}

	return id, nil
}

func (r *PostgresRepository) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	user := &models.User{}
	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, login, password_hash, created_at FROM users WHERE login = $1",
		login,
	).Scan(&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return user, nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	user := &models.User{}
	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, login, password_hash, created_at FROM users WHERE id = $1",
		id,
	).Scan(&user.ID, &user.Login, &user.PasswordHash, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return user, nil
}

func (r *PostgresRepository) CreateOrder(ctx context.Context, userID int64, orderNumber string) error {
	_, err := r.db.ExecContext(
		ctx,
		"INSERT INTO orders (user_id, number, status) VALUES ($1, $2, $3)",
		userID, orderNumber, models.StatusNew,
	)
	return err
}

func (r *PostgresRepository) GetOrderByNumber(ctx context.Context, orderNumber string) (*models.Order, error) {
	order := &models.Order{}
	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, number, user_id, status, accrual, uploaded_at FROM orders WHERE number = $1",
		orderNumber,
	).Scan(&order.ID, &order.Number, &order.UserID, &order.Status, &order.Accrual, &order.UploadedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	return order, nil
}

func (r *PostgresRepository) GetUserOrders(ctx context.Context, userID int64) ([]models.Order, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, number, user_id, status, accrual, uploaded_at 
         FROM orders 
         WHERE user_id = $1
         ORDER BY uploaded_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		if err := rows.Scan(
			&order.ID,
			&order.Number,
			&order.UserID,
			&order.Status,
			&order.Accrual,
			&order.UploadedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *PostgresRepository) UpdateOrderStatus(ctx context.Context, orderNumber, status string, accrual float64) error {
	_, err := r.db.ExecContext(
		ctx,
		"UPDATE orders SET status = $1, accrual = $2 WHERE number = $3",
		status, accrual, orderNumber,
	)
	return err
}

func (r *PostgresRepository) GetUserBalance(ctx context.Context, userID int64) (*models.Balance, error) {
	balance := &models.Balance{}

	err := r.db.QueryRowContext(
		ctx,
		`SELECT 
            COALESCE(SUM(accrual), 0) 
         FROM orders 
         WHERE user_id = $1 AND status = $2`,
		userID, models.StatusProcessed,
	).Scan(&balance.Current)
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(
		ctx,
		`SELECT 
            COALESCE(SUM(sum), 0) 
         FROM withdrawals 
         WHERE user_id = $1`,
		userID,
	).Scan(&balance.Withdrawn)
	if err != nil {
		return nil, err
	}

	balance.Current -= balance.Withdrawn

	return balance, nil
}

func (r *PostgresRepository) WithdrawBalance(ctx context.Context, userID int64, orderNumber string, amount float64) error {
	balance, err := r.GetUserBalance(ctx, userID)
	if err != nil {
		return err
	}

	if balance.Current < amount {
		return errors.New("insufficient funds")
	}

	_, err = r.db.ExecContext(
		ctx,
		"INSERT INTO withdrawals (user_id, order_number, sum, processed_at) VALUES ($1, $2, $3, $4)",
		userID, orderNumber, amount, time.Now(),
	)
	return err
}

func (r *PostgresRepository) GetUserWithdrawals(ctx context.Context, userID int64) ([]models.Withdrawal, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, user_id, order_number, sum, processed_at 
         FROM withdrawals 
         WHERE user_id = $1
         ORDER BY processed_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []models.Withdrawal
	for rows.Next() {
		var w models.Withdrawal
		var orderNumber string
		if err := rows.Scan(
			&w.ID,
			&w.UserID,
			&orderNumber,
			&w.Sum,
			&w.ProcessedAt,
		); err != nil {
			return nil, err
		}
		w.Order = orderNumber
		withdrawals = append(withdrawals, w)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return withdrawals, nil
}
