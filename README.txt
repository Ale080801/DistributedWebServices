GoFishing: Microservices E-Commerce Platform
=================================================

GoFishing is a distributed e-commerce platform dedicated to fishing gear, built as a 
final project for the Distributed Programming course. 
It implements a Microservices Architecture using Go, gRPC, and SQLite.

ARCHITECTURE OVERVIEW
---------------------
The system is composed of an API Gateway and four distinct, loosely coupled microservices:
- API Gateway (cmd/api-gateway): The single entry point. Serves the frontend 
  (Server-Side Rendering + Vanilla JS) and acts as a REST-to-gRPC translator 
  for client requests.
- Auth Service (cmd/auth-service): Manages user registration, secure login (bcrypt),
  and stateless JWT authentication.
- Catalog Service (cmd/catalog-service): Manages the product catalog with soft-delete 
  capabilities.
- Inventory Service (cmd/inventory-service): Handles stock quantities with atomic 
  concurrency controls.
- Orders Service (cmd/orders-service): Orchestrates checkout, utilizing a Saga pattern
  (compensation transactions) to manage distributed state between orders and inventory.


PREREQUISITES
-------------
To compile and run this project, you need:
- Go: Version 1.20 or higher installed.
- C Compiler (GCC): Required because the project uses 'go-sqlite3' which relies on CGO.
  * Windows: Install MinGW-w64 or TDM-GCC.
  * Linux: Install build-essential or gcc.


PORTS IN USE
------------
The system utilizes the following ports locally(localhost):
- 8080  (HTTP) : API Gateway (Web Interface & REST API)
- 50051 (gRPC) : Catalog Service
- 50052 (gRPC) : Auth Service
- 50053 (gRPC) : Inventory Service
- 50054 (gRPC) : Orders Service


COMPILATION AND EXECUTION (DEPLOYMENT)
--------------------------------------
The project includes automated scripts that build the binaries and start all the 
microservices in the background, followed by the API Gateway. 
(Note: 'go run' automatically compiles the code into a temporary executable).

[ Manual Compilation ]
If you prefer to compile the binaries manually before running them, you can do so
from the root of the project:
    go build -o bin/api-gateway.exe ./cmd/api-gateway
    go build -o bin/catalog-service.exe ./cmd/catalog-service
    go build -o bin/auth-service.exe ./cmd/auth-service
    go build -o bin/inventory-service.exe ./cmd/inventory-service
    go build -o bin/orders-service.exe ./cmd/orders-service

[ Automated Startup via Scripts ]

[ Windows ]
Open PowerShell as Administrator (or ensure script execution is enabled) and run:
    .\scripts\start-all.ps1

To cleanly stop all the microservices:
    .\scripts\stop-all.ps1

[ Linux / macOS ]
Grant execution permissions and run the bash scripts:
    chmod +x scripts/*.sh
    ./scripts/start-all.sh

To cleanly stop all the microservices:
    ./scripts/stop-all.sh

Once started, the platform is accessible via your web browser at:
http://localhost:8080


DEFAULT ACCOUNTS
----------------
The database comes pre-seeded with some default accounts for testing purposes:

Admin Account:
  Email: admin@test.it
  Password: admin

Standard User Account:
  Email: test@test.it
  Password: test


RUNNING TESTS
-------------
The project features a comprehensive suite of Unit and Integration Tests using in-memory 
databases and mock gRPC servers.
To run the entire test suite, execute the following command from the root directory:

    go test ./... -v -count=1


PROJECT STRUCTURE
-----------------
- cmd/ : Contains the main.go entry points for the API Gateway and the four microservices.
- internal/ : Contains the core logic.
    - proto/ : Protobuf definitions (.proto files) and generated gRPC code.
    - database/ : Service-specific SQLite database initialization and queries.
    - gateway/ : REST API handlers and Web (SSR) handlers.
- templates/ : HTML templates rendered server-side by the API Gateway.
- scripts/ : Deployment and automation scripts for Windows and Linux.
- test/ : Integration tests.
- data/ : (Generated at runtime) Contains the isolated SQLite .db files for each service.
