import "./assets/main.css";

import {createApp} from "vue";
import App from "./views/App.vue";

// element-plus
import ElementPlus from "element-plus";
import "element-plus/dist/index.css";



createApp(App).use(ElementPlus).mount("#app");
