import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { watchInputModality } from './lib/shortcuts';
import { initTheme } from './lib/theme.svelte';

const target = document.getElementById('app');
if (!target) throw new Error('index.html is missing <div id="app">');

initTheme();
watchInputModality();

export default mount(App, { target });
