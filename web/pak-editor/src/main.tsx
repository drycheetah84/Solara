import React from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import { ApiContext } from './apiContext';
import { createAppApi, MOCK_MODE } from './apiInstance';
import { initSessionToken } from './session';
import './styles.css';

const token = initSessionToken();
const api = createAppApi();

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <ApiContext.Provider value={api}>
      <App hasToken={MOCK_MODE || token !== null} mock={MOCK_MODE} />
    </ApiContext.Provider>
  </React.StrictMode>,
);
