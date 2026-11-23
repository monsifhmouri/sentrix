# Examples

    sentrix scan -name acme -domain example.com -scope exclude:dev.example.com
    sentrix scan -name acme2 -domain example.com -scope host:api.partner.test -scope url:https://shared.test/acme \
        -sources crtsh,wayback -rate 0.5 -concurrency 2 -format json,csv,html
    sentrix show -name acme -class graphql -scope in
    sentrix export -name acme -format html -out ./reports
    sentrix backup -name acme -to acme.bak && sentrix restore -from acme.bak -overwrite
