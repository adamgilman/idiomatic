import React, { useState, useEffect } from "react";

interface Item {
  id: string;
  name: string;
}

interface AppProps {
  items: Item[];
  isAdmin: boolean;
}

export function App({ items, isAdmin }: AppProps): React.JSX.Element {
  const [count, setCount] = useState(0);

  // Violation 1: react/jsx-key — missing key prop in .map()
  const itemList = items.map((item) => <li>{item.name}</li>);

  // Violation 2: react-hooks/rules-of-hooks — hook called conditionally
  if (isAdmin) {
    useEffect(() => {
      document.title = `Admin: ${count}`;
    }, [count]);
  }

  return (
    <div>
      <button onClick={() => setCount(count + 1)}>Count: {count}</button>
      <ul>{itemList}</ul>
    </div>
  );
}

export default App;
