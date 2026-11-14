for (const button of document.querySelectorAll('.cmd button')) {
  button.addEventListener('click', async () => {
    const text = button.parentElement.querySelector('code').textContent;
    try {
      await navigator.clipboard.writeText(text);
      button.textContent = 'Copied';
    } catch {
      button.textContent = 'Select it';
    }
    setTimeout(() => { button.textContent = 'Copy'; }, 1600);
  });
}
