# Local outbound gRPC mTLS materials

Place the shared Parstis workstation CA and BNPL client identity here (never commit the PEM files):

```text
assets/local-development/certs/ca.crt
assets/local-development/certs/bnpl-service.crt
assets/local-development/certs/bnpl-service.key
```

These are used by BNPL when `EXTERNAL_*_GRPC_INSECURE=false` for Financial Core,
Customer Reference, and Notification. Obtain the files from the FC / platform team.

Wire them via `.env` (see `.env.example`):

```env
EXTERNAL_FINANCIAL_CORE_GRPC_TLS_CA_FILE=./assets/local-development/certs/ca.crt
EXTERNAL_FINANCIAL_CORE_GRPC_TLS_CERT_FILE=./assets/local-development/certs/bnpl-service.crt
EXTERNAL_FINANCIAL_CORE_GRPC_TLS_KEY_FILE=./assets/local-development/certs/bnpl-service.key
```

(and the matching `EXTERNAL_CUSTOMER_*` / `EXTERNAL_NOTIFICATION_*` paths).
