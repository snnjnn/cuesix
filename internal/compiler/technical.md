# Compiler: detalles tecnicos

Entradas:

- Uno o mas `fs.FS` con ficheros YAML.
- Reglas de fusion definidas por `MergingRule`.

Salidas:

- Mapa `map[string]any` con la configuracion combinada.
- Error si hay problemas de lectura, parsing o conflictos de fusion.
- El mapa resultante puede ser transformado por plugins antes de serializarse.

Dependencias:

- `io/fs` para el acceso a los sistemas de ficheros.
- `go.yaml.in/yaml/v4` para parsear YAML.

Reglas y fusion:

- Las listas se fusionan solo si existe una regla con `Kind: list`.
- Listas con clave obligatoria deben incluirla; si falta, se devuelve error.
- Listas con clave opcional no generan ids: los elementos sin clave se preservan sin fusion.
- Si dos elementos comparten clave, se fusionan si `AllowMergeSameID` es true; en caso contrario, error.
- La fusion profunda exige igualdad de escalares o presencia en un solo lado; mapas se fusionan recursivamente; listas requieren regla.
