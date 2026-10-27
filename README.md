# Tyche: Fortune Builder

## How to Run

To run the application, first copy the `.env.example` file to `.env`
and fill in the information if needed.

Make sure you have `data` and `logs` directories

```bash
mkdir data logs
```

Create you `.env` file

```bash
cp .env.example .env
```

Fetch the [simple-icons](https://github.com/simple-icons/simple-icons) submodule (brand icons for counterparties)

```bash
git submodule update --init --depth 1
```

Install node dev dependencies

```bash
npm install
```

To start the application on development mode run

```bash
make dev
```

Or to start in production mode run:

```bash
make build
make start
```
