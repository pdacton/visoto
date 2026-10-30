// Mounts Swagger UI on every [data-api-docs] element, reading the OpenAPI
// document from its data-spec-url. Markup: templates/pages/apiDocs.html.
document.querySelectorAll('[data-api-docs]').forEach((el) => {
  if (!window.SwaggerUIBundle) return;
  SwaggerUIBundle({
    domNode: el,
    url: el.dataset.specUrl,
    deepLinking: true,
    docExpansion: 'list',
    defaultModelsExpandDepth: 0,
    displayRequestDuration: true,
    filter: true,
    tryItOutEnabled: false,
  });
});
