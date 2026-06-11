// Mirror of server-java com.acme.gateway.Routes. Kept in sync by hand;
// the version skew (client 2.2.7 vs server 2.3.0) is why entries drift.
export const API_ROUTES = {
  search: { method: "GET", path: "/v1/search" },
  getDoc: { method: "GET", path: "/v1/docs/{id}" },
  putDoc: { method: "PUT", path: "/v1/docs/{id}" },
  deleteDoc: { method: "DELETE", path: "/v1/docs/{id}" },
} as const;
