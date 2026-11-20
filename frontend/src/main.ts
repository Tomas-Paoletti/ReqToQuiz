import { createApp } from "vue";
import { createRouter, createWebHistory } from "vue-router";
import App from "./App.vue";
import HomeView from "./views/HomeView.vue";
import GeneratingView from "./views/GeneratingView.vue";
import CvEditorView from "./views/CvEditorView.vue";
import "./style.css";

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", component: HomeView },
    { path: "/generating/:id", component: GeneratingView, props: true },
    { path: "/cv/:id", component: CvEditorView, props: true },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
});

createApp(App).use(router).mount("#app");
