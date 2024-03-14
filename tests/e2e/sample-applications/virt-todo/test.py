#!/usr/bin/env python

import argparse
import json
import requests

from datetime import datetime


def updateToDo(base_url, id, completed):
  """Update data to the todo application

  Args:
    item_dict: dict of todo item
    completed: bool

  Returns:
    bool
  """

  # Set the endpoint URL
  endpoint = base_url + "/todo/" + str(id)
  # Send a POST request with the data and the endpoint URL
  response = requests.post(endpoint, data=data)
  # Check the status code of the response
  if response.status_code == 201 or response.status_code == 200:
      print("Task updated successfully!")
      return True
  else:
      print("Error updating task.")
      return False
  

def createToDo(base_url, description, completed):
  """Post data to the todo application

  Args:
    description: todo list description
    completed: bool

  Returns:
    id of todo item in db
  """
  data = {
      "description": description,
      "completed": completed
  }

  # Set the endpoint URL
  endpoint = base_url + "/todo"
  # Send a POST request with the data and the endpoint URL
  response = requests.post(endpoint, data=data)
  # Check the status code of the response
  if response.status_code == 201 or response.status_code == 200:
      print("Task created successfully!")
  else:
      print("Error creating task.")
  response_dict = json.loads(response.text)[0]
  return response_dict

def checkToDoLists(base_url, completed):
  """Get data from the todo application

  Args:
    completed: bool

  Returns:
    json dict
  """
  # Set the endpoint URL
  if completed:
    endpoint = base_url + "/todo-completed"
  else:
    endpoint = base_url + "/todo-incomplete"
  # Send a POST request with the data and the endpoint URL
  response = requests.get(endpoint)
  # Check the status code of the response
  if response.status_code == 201 or response.status_code == 200:
      print("Got list of items")
  else:
      print("Failed to get list of items")
  response_dict = json.loads(response.text)
  return response_dict

def deleteToDoItems(base_url, item):
  """Delete data from the todo application

  Args:
    item: dict

  Returns:
    bool
  """

  endpoint = base_url + "/todo/" + str(item["Id"])
  # Send a POST request with the data and the endpoint URL
  response = requests.delete(endpoint)
  # Check the status code of the response
  if response.status_code == 201 or response.status_code == 200:
      print("Deleted item " + str(item["Id"]))
      return True
  else:
      print("Failed to delete item " + str(item["Id"]))
      return False

def main():
  parser = argparse.ArgumentParser(description='Process some integers.')
  parser.add_argument('--base_url', dest='base_url', required=True,
                      help='The openshift route to the VM')
  args = parser.parse_args()
  print(args.base_url)
  base_url = args.base_url


  date = datetime.today().strftime('%Y-%m-%d-%H:%M:%S')
  # create todo items
  test1 = createToDo(base_url, "pytest-1-" + date, False)
  test2 = createToDo(base_url, "pytest-2-" + date, False)
  test3 = createToDo(base_url, "pytest-1-" + date, False)

  # update todo items
  success = updateToDo(base_url, test1["Id"], True)
  success = updateToDo(base_url, test2["Id"], True)

  # check todo's
  completed = checkToDoLists(base_url, True)
  incomplete = checkToDoLists(base_url, False)
  print("COMPLETED ITEMS:")
  print(completed)
  print("INCOMPLETE ITEMS:")
  print(incomplete)

  # test complete or incomplete
  found_completed = False
  for i in completed:
    if test1["Description"] == i["Description"]:
      found_completed = True

  found_incomplete = False
  for i in incomplete:
    if test3["Description"] == i["Description"]:
      found_incomplete = True
   
  if found_completed == False or found_incomplete == False:
    print("FAILED complete / incomplete TEST")
  else:
    print("SUCCESS!")

  # Delete items
  deleteToDoItems(base_url, test1)
  deleteToDoItems(base_url, test3)
  completed = checkToDoLists(base_url, True)
  incomplete = checkToDoLists(base_url, False)
  print("COMPLETED ITEMS:")
  print(completed)
  print("INCOMPLETE ITEMS:")
  print(incomplete)

  # Test deleted items
  found_completed = False
  for i in completed:
    if test1["Description"] == i["Description"]:
      found_completed = True

  found_incomplete = False
  for i in incomplete:
    if test3["Description"] == i["Description"]:
      found_incomplete = True
    
  if found_completed == True or found_incomplete == True:
    print("FAILED Delete TEST")
  else:
    print("SUCCESS!")
   


if __name__ == "__main__":
    main()

