import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ObserverDashboard } from "../src";
import { LocalHttpObserverDataSource } from "../src/local";
import "../src/styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("Dashboard root is missing");

createRoot(root).render(
  <StrictMode>
    <ObserverDashboard dataSource={new LocalHttpObserverDataSource({ baseUrl: "" })} mode="local" />
  </StrictMode>
);
