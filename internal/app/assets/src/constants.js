export const ENDPOINTS = {
  validate: '../validate',
  sources: '../sources',
  gateways: '../virtualgw',
  virtualgw: '../virtualgw'
};

export const SAMPLE_YAML = `ssls:
  - id: cert0
    cert: "$secret://file/customer1/cert.pem"
    key: "$secret://file/customer1/key.pem"
    snis:
      - "example.com"
routes:
  - id: sample
    uri: /hello
    upstream:
      type: roundrobin
      nodes:
        "\${{ UPSTREAM_URL:=127.0.0.1:8080 }}": 1
    plugins:
      prometheus: {}
`;

export const SAMPLE_SOURCE = {
  value: '__sample__',
  label: 'Sample: inline.yml',
  content: SAMPLE_YAML
};

export const MODES = {
  browse: 'browse',
  index: 'index',
  playground: 'playground'
};
