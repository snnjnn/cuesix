import 'bulma/css/bulma.min.css';
import '@fortawesome/fontawesome-free/css/all.min.css';
import 'datatables.net-bm/css/dataTables.bulma.min.css';
import 'datatables.net-bm';
import './theme-overrides.css';

import { registerAlpineSchemaApp } from './alpineApp.js';

document.addEventListener('DOMContentLoaded', registerAlpineSchemaApp);
